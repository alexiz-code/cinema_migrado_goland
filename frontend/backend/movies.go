package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type movie struct {
	ID          int64
	Title       string
	Synopsis    string
	Year        int
	Duration    int
	RatingLabel string
	Genres      string
	Poster      string
	Price       float64
	Rating      float64
	Popularity  float64
	State       string
	CreatedAt   string
}

func (s *server) loadMovie(id int64) (movie, error) {
	var m movie
	err := s.db.QueryRow(`SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE id=?`, id).
		Scan(&m.ID, &m.Title, &m.Synopsis, &m.Year, &m.Duration, &m.RatingLabel, &m.Genres, &m.Poster, &m.Price, &m.Rating, &m.Popularity, &m.State, &m.CreatedAt)
	return m, err
}

func (s *server) movieCounts(id int64) (int, int, error) {
	var total, available int
	err := s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN estado='DISPONIBLE' THEN 1 ELSE 0 END),0) FROM copias WHERE pelicula_id=?`, id).
		Scan(&total, &available)
	if available < 0 {
		available = 0
	}
	return total, available, err
}

func (s *server) movieJSON(m movie) (map[string]any, error) {
	total, available, err := s.movieCounts(m.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": m.ID, "titulo": m.Title, "sinopsis": m.Synopsis, "anio": m.Year,
		"duracion": m.Duration, "clasificacion": m.RatingLabel, "generos": m.Genres,
		"poster_url": m.Poster, "precio_dia": m.Price, "rating": m.Rating,
		"popularidad": m.Popularity, "estado": m.State, "fecha_creacion": parseTime(m.CreatedAt),
		"copias_totales": total, "copias_disponibles": available,
	}, nil
}

func (s *server) listMovies(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE 1=1`
	args := []any{}
	includeInactive := r.URL.Query().Get("incluir_inactivas") == "true"
	if includeInactive {
		if u, ok := s.optionalUser(r); !ok || u.Role != "ADMIN" {
			includeInactive = false
		}
	}
	if !includeInactive {
		query += ` AND estado='ACTIVO'`
	}
	if title := strings.TrimSpace(r.URL.Query().Get("titulo")); title != "" {
		query += ` AND lower(titulo) LIKE ?`
		args = append(args, "%"+strings.ToLower(title)+"%")
	}
	if genre := strings.TrimSpace(r.URL.Query().Get("genero")); genre != "" {
		query += ` AND lower(generos) LIKE ?`
		args = append(args, "%"+strings.ToLower(genre)+"%")
	}
	query += ` ORDER BY popularidad DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("list movies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
		return
	}
	movies := []movie{}
	for rows.Next() {
		var m movie
		if err := rows.Scan(&m.ID, &m.Title, &m.Synopsis, &m.Year, &m.Duration, &m.RatingLabel, &m.Genres, &m.Poster, &m.Price, &m.Rating, &m.Popularity, &m.State, &m.CreatedAt); err != nil {
			_ = rows.Close()
			fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
			return
		}
		movies = append(movies, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate movies: %v", err)
		_ = rows.Close()
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("close movie rows: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
		return
	}
	result := []map[string]any{}
	for _, m := range movies {
		item, err := s.movieJSON(m)
		if err != nil {
			log.Printf("count copies for movie %d: %v", m.ID, err)
			fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
			return
		}
		if r.URL.Query().Get("disponible") == "true" && item["copias_disponibles"].(int) == 0 {
			continue
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) movieByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	m, err := s.loadMovie(id)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Película no encontrada")
		return
	}
	if err != nil {
		log.Printf("load movie: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	response, err := s.movieJSON(m)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) movieAvailability(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	m, err := s.loadMovie(id)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Película no encontrada")
		return
	}
	if err != nil {
		log.Printf("load movie availability: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo consultar la disponibilidad.")
		return
	}
	rows, err := s.db.Query(`SELECT id,pelicula_id,codigo_inventario,estado,ubicacion,observaciones FROM copias WHERE pelicula_id=?`, id)
	if err != nil {
		log.Printf("list movie copies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo consultar la disponibilidad.")
		return
	}
	defer rows.Close()
	copies := []map[string]any{}
	available := 0
	for rows.Next() {
		c, err := scanCopy(rows)
		if err != nil {
			fail(w, http.StatusInternalServerError, "No se pudo consultar la disponibilidad.")
			return
		}
		c["titulo_pelicula"] = m.Title
		if c["estado"] == "DISPONIBLE" {
			available++
		}
		copies = append(copies, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pelicula_id": m.ID, "titulo": m.Title, "precio_dia": m.Price,
		"total_copias": len(copies), "disponibles": available, "copias": copies,
	})
}

func (s *server) createMovie(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	var input map[string]any
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de película inválidos.")
		return
	}
	title := stringField(input, "titulo")
	synopsis := stringField(input, "sinopsis")
	year := intField(input, "anio", 0)
	duration := intField(input, "duracion", 0)
	label := stringField(input, "clasificacion")
	if label == "" {
		label = "PG-13"
	}
	genres := stringField(input, "generos")
	poster := stringField(input, "poster_url")
	price := floatField(input, "precio_dia", 0)
	rating := floatField(input, "rating", 7)
	popularity := floatField(input, "popularidad", 80)
	copies := intField(input, "copias_iniciales", 0)
	if len(title) < 1 || len(title) > 180 || len(synopsis) < 1 || year < 1890 || year > 2050 ||
		duration < 1 || len(label) > 20 || len(genres) < 1 || len(genres) > 200 ||
		len(poster) < 1 || len(poster) > 500 || !(strings.HasPrefix(poster, "http://") || strings.HasPrefix(poster, "https://")) ||
		price <= 0 || rating < 0 || rating > 10 || popularity < 0 || copies < 0 {
		fail(w, http.StatusUnprocessableEntity, "Datos de película inválidos.")
		return
	}
	now := time.Now().UTC().Format(dateLayout)
	result, err := s.db.Exec(`INSERT INTO peliculas(titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(title), strings.TrimSpace(synopsis), year, duration, label, genres, poster, price, rating, popularity, "ACTIVO", now)
	if err != nil {
		log.Printf("create movie: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo crear la película.")
		return
	}
	id, _ := result.LastInsertId()
	for i := 1; i <= copies; i++ {
		code := fmt.Sprintf("CP-%04d-%02d", id, i)
		if _, err := s.db.Exec(`INSERT INTO copias(pelicula_id,codigo_inventario,estado,ubicacion) VALUES(?,?,?,?)`, id, code, "DISPONIBLE", fmt.Sprintf("Estante P-%d", id)); err != nil {
			log.Printf("create initial copy: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudieron crear las copias iniciales.")
			return
		}
	}
	m, err := s.loadMovie(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película creada.")
		return
	}
	response, err := s.movieJSON(m)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película creada.")
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *server) updateMovie(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	if _, err := s.loadMovie(id); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Película no encontrada")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	var raw map[string]json.RawMessage
	if decodeBody(r, &raw) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de película inválidos.")
		return
	}
	allowed := map[string]string{
		"titulo": "titulo", "sinopsis": "sinopsis", "anio": "anio", "duracion": "duracion",
		"clasificacion": "clasificacion", "generos": "generos", "poster_url": "poster_url",
		"precio_dia": "precio_dia", "rating": "rating", "popularidad": "popularidad",
	}
	sets := []string{}
	args := []any{}
	for key, value := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if string(value) == "null" {
			fail(w, http.StatusUnprocessableEntity, "Los campos enviados para actualizar no pueden ser nulos.")
			return
		}
		var v any
		if err := json.Unmarshal(value, &v); err != nil {
			fail(w, http.StatusUnprocessableEntity, "Campo inválido.")
			return
		}
		if !validMovieField(key, v) {
			fail(w, http.StatusUnprocessableEntity, "Datos de película inválidos.")
			return
		}
		sets = append(sets, allowed[key]+"=?")
		args = append(args, v)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.Exec(`UPDATE peliculas SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
			log.Printf("update movie: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudo actualizar la película.")
			return
		}
	}
	m, err := s.loadMovie(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	response, err := s.movieJSON(m)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func validMovieField(key string, value any) bool {
	switch key {
	case "titulo":
		v, ok := value.(string)
		return ok && len(v) > 0 && len(v) <= 180
	case "sinopsis":
		v, ok := value.(string)
		return ok && len(v) > 0
	case "anio":
		v := numeric(value)
		return v >= 1890 && v <= 2050 && v == float64(int(v))
	case "duracion":
		return numeric(value) > 0 && numeric(value) == float64(int(numeric(value)))
	case "clasificacion":
		v, ok := value.(string)
		return ok && len(v) > 0 && len(v) <= 20
	case "generos":
		v, ok := value.(string)
		return ok && len(v) > 0 && len(v) <= 200
	case "poster_url":
		v, ok := value.(string)
		return ok && len(v) > 0 && len(v) <= 500
	case "precio_dia":
		return numeric(value) > 0
	case "rating":
		return numeric(value) >= 0 && numeric(value) <= 10
	case "popularidad":
		return numeric(value) >= 0
	default:
		return false
	}
}

func (s *server) setMovieState(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var input struct {
		State string `json:"estado"`
	}
	if decodeBody(r, &input) != nil || (input.State != "ACTIVO" && input.State != "INACTIVO") {
		fail(w, http.StatusUnprocessableEntity, "Estado inválido.")
		return
	}
	result, err := s.db.Exec(`UPDATE peliculas SET estado=? WHERE id=?`, input.State, id)
	if err != nil {
		log.Printf("set movie state: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo cambiar el estado.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		fail(w, http.StatusNotFound, "Película no encontrada")
		return
	}
	m, err := s.loadMovie(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	response, err := s.movieJSON(m)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la película.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) optionalUser(r *http.Request) (user, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return user{}, false
	}
	id, err := s.parseToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return user{}, false
	}
	u, err := s.findUser(id)
	return u, err == nil && u.State == "ACTIVO"
}

type copyRow interface{ Scan(...any) error }

func scanCopy(row copyRow) (map[string]any, error) {
	var id, movieID int64
	var code, state string
	var location, observations sql.NullString
	err := row.Scan(&id, &movieID, &code, &state, &location, &observations)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "pelicula_id": movieID, "codigo_inventario": code, "estado": state,
		"ubicacion": nullValue(location), "observaciones": nullValue(observations),
	}, nil
}

func (s *server) listCopies(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	query := `SELECT c.id,c.pelicula_id,c.codigo_inventario,c.estado,c.ubicacion,c.observaciones,p.titulo FROM copias c JOIN peliculas p ON p.id=c.pelicula_id WHERE 1=1`
	args := []any{}
	if raw := r.URL.Query().Get("pelicula_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "pelicula_id inválido.")
			return
		}
		query += ` AND c.pelicula_id=?`
		args = append(args, id)
	}
	if state := r.URL.Query().Get("estado"); state != "" {
		query += ` AND c.estado=?`
		args = append(args, state)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("list copies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las copias.")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var c map[string]any
		var id, movieID int64
		var code, state, title string
		var location, notes sql.NullString
		if err := rows.Scan(&id, &movieID, &code, &state, &location, &notes, &title); err != nil {
			fail(w, http.StatusInternalServerError, "No se pudieron cargar las copias.")
			return
		}
		c = map[string]any{"id": id, "pelicula_id": movieID, "codigo_inventario": code, "estado": state,
			"ubicacion": nullValue(location), "observaciones": nullValue(notes), "titulo_pelicula": title}
		result = append(result, c)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) createCopy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	var input struct {
		MovieID  int64   `json:"pelicula_id"`
		Code     string  `json:"codigo_inventario"`
		Location *string `json:"ubicacion"`
		Notes    *string `json:"observaciones"`
	}
	if decodeBody(r, &input) != nil || input.MovieID < 1 || strings.TrimSpace(input.Code) == "" {
		fail(w, http.StatusUnprocessableEntity, "Datos de copia inválidos.")
		return
	}
	if _, err := s.loadMovie(input.MovieID); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Película asociada no encontrada")
		return
	} else if err != nil {
		log.Printf("load associated movie: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo validar la película asociada.")
		return
	}
	code := strings.TrimSpace(input.Code)
	if _, err := s.db.Exec(`INSERT INTO copias(pelicula_id,codigo_inventario,estado,ubicacion,observaciones) VALUES(?,?,?,?,?)`,
		input.MovieID, code, "DISPONIBLE", stringOrDefault(input.Location, "Estante General"), input.Notes); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			fail(w, http.StatusBadRequest, "Ya existe una copia con este código de inventario")
			return
		}
		log.Printf("create copy: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo crear la copia.")
		return
	}
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM copias WHERE codigo_inventario=?`, code).Scan(&id)
	result, err := s.copyByID(id)
	if err != nil {
		log.Printf("load newly created copy: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo cargar la copia creada.")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) copyByID(id int64) (map[string]any, error) {
	var movieTitle string
	var c map[string]any
	var copyID, movieID int64
	var code, state string
	var location, notes sql.NullString
	err := s.db.QueryRow(`SELECT c.id,c.pelicula_id,c.codigo_inventario,c.estado,c.ubicacion,c.observaciones,p.titulo FROM copias c JOIN peliculas p ON p.id=c.pelicula_id WHERE c.id=?`, id).
		Scan(&copyID, &movieID, &code, &state, &location, &notes, &movieTitle)
	if err != nil {
		return nil, err
	}
	c = map[string]any{"id": copyID, "pelicula_id": movieID, "codigo_inventario": code,
		"estado": state, "ubicacion": nullValue(location), "observaciones": nullValue(notes), "titulo_pelicula": movieTitle}
	return c, nil
}

func (s *server) updateCopy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var state, current string
	if err := s.db.QueryRow(`SELECT estado FROM copias WHERE id=?`, id).Scan(&current); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Copia no encontrada")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la copia.")
		return
	}
	var raw map[string]json.RawMessage
	if decodeBody(r, &raw) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de copia inválidos.")
		return
	}
	var fields map[string]any
	if err := json.Unmarshal(mustJSON(raw), &fields); err != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de copia inválidos.")
		return
	}
	sets, args := []string{}, []any{}
	if value, ok := fields["estado"]; ok {
		state, _ = value.(string)
		valid := map[string]bool{"DISPONIBLE": true, "ALQUILADA": true, "MANTENIMIENTO": true, "DADA_DE_BAJA": true}
		if !valid[state] {
			fail(w, http.StatusUnprocessableEntity, "Estado inválido.")
			return
		}
		if state == "ALQUILADA" && current != "ALQUILADA" {
			fail(w, http.StatusBadRequest, "Una copia solo puede marcarse como alquilada al crear un alquiler.")
			return
		}
		if current == "ALQUILADA" && state != "ALQUILADA" {
			fail(w, http.StatusBadRequest, "Registre la devolución del alquiler antes de cambiar el estado de esta copia.")
			return
		}
		sets, args = append(sets, "estado=?"), append(args, state)
	}
	for key, column := range map[string]string{"codigo_inventario": "codigo_inventario", "ubicacion": "ubicacion", "observaciones": "observaciones"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if key == "codigo_inventario" {
			text, valid := value.(string)
			if !valid || strings.TrimSpace(text) == "" {
				fail(w, http.StatusUnprocessableEntity, "Código de inventario inválido.")
				return
			}
			value = strings.TrimSpace(text)
		}
		sets, args = append(sets, column+"=?"), append(args, value)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.Exec(`UPDATE copias SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				fail(w, http.StatusBadRequest, "Ya existe una copia con este código de inventario")
				return
			}
			log.Printf("update copy: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudo actualizar la copia.")
			return
		}
	}
	c, err := s.copyByID(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar la copia.")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func stringOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func stringField(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func numeric(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return -1
	}
}

func intField(data map[string]any, key string, fallback int) int {
	value, ok := data[key]
	if !ok {
		return fallback
	}
	number := numeric(value)
	if number != float64(int(number)) {
		return -1
	}
	return int(number)
}

func floatField(data map[string]any, key string, fallback float64) float64 {
	value, ok := data[key]
	if !ok {
		return fallback
	}
	return numeric(value)
}

func (s *server) compatMovies(w http.ResponseWriter, r *http.Request) {
	s.compatMovieList(w, r, `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE estado='ACTIVO'`)
}

func (s *server) compatTrends(w http.ResponseWriter, r *http.Request) {
	s.compatMovieList(w, r, `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE estado='ACTIVO' ORDER BY popularidad DESC LIMIT 10`)
}

func (s *server) compatTopRated(w http.ResponseWriter, r *http.Request) {
	s.compatMovieList(w, r, `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE estado='ACTIVO' ORDER BY rating DESC LIMIT 10`)
}

func (s *server) compatGenre(w http.ResponseWriter, r *http.Request) {
	genre := strings.ToLower(strings.TrimSpace(r.PathValue("genero")))
	s.compatMovieListArgs(w, r, `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE estado='ACTIVO' AND lower(generos) LIKE ?`, "%"+genre+"%")
}

func (s *server) compatSearch(w http.ResponseWriter, r *http.Request) {
	term := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	query := `SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE estado='ACTIVO'`
	if term != "" {
		query += ` AND (lower(titulo) LIKE ? OR lower(generos) LIKE ? OR lower(sinopsis) LIKE ?)`
		filter := "%" + term + "%"
		s.compatMovieListArgs(w, r, query, filter, filter, filter)
		return
	}
	s.compatMovieList(w, r, query)
}

func (s *server) compatRecommend(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tastes []string `json:"gustos"`
	}
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de recomendación inválidos.")
		return
	}
	movies, err := s.allMovies(`WHERE estado='ACTIVO'`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las recomendaciones.")
		return
	}
	tastes := []string{}
	for _, taste := range input.Tastes {
		if taste = strings.ToLower(strings.TrimSpace(taste)); taste != "" {
			tastes = append(tastes, taste)
		}
	}
	sort.SliceStable(movies, func(i, j int) bool {
		left, right := countMatches(movies[i].Genres, tastes), countMatches(movies[j].Genres, tastes)
		if left != right {
			return left > right
		}
		return movies[i].Rating > movies[j].Rating
	})
	if len(tastes) == 0 {
		movies = movies[:minInt(6, len(movies))]
	} else {
		filtered := []movie{}
		for _, m := range movies {
			if countMatches(m.Genres, tastes) > 0 {
				filtered = append(filtered, m)
			}
		}
		movies = filtered
		if len(movies) == 0 {
			movies, err = s.allMovies(`WHERE estado='ACTIVO'`)
			if err != nil {
				fail(w, http.StatusInternalServerError, "No se pudieron cargar las recomendaciones.")
				return
			}
			movies = movies[:minInt(6, len(movies))]
		} else {
			movies = movies[:minInt(8, len(movies))]
		}
	}
	s.writeCompatMovies(w, movies)
}

func (s *server) compatSimilar(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title string `json:"titulo"`
	}
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Título inválido.")
		return
	}
	target, err := s.db.Query(`SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas WHERE lower(titulo) LIKE ? LIMIT 1`, "%"+strings.ToLower(strings.TrimSpace(input.Title))+"%")
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudieron cargar películas similares.")
		return
	}
	var selected []movie
	for target.Next() {
		var m movie
		if err := target.Scan(&m.ID, &m.Title, &m.Synopsis, &m.Year, &m.Duration, &m.RatingLabel, &m.Genres, &m.Poster, &m.Price, &m.Rating, &m.Popularity, &m.State, &m.CreatedAt); err == nil {
			selected = append(selected, m)
		}
	}
	target.Close()
	movies, err := s.allMovies(`WHERE estado='ACTIVO'`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudieron cargar películas similares.")
		return
	}
	if len(selected) == 0 {
		movies = movies[:minInt(6, len(movies))]
	} else {
		genres := splitGenres(selected[0].Genres)
		filtered := []movie{}
		for _, m := range movies {
			if m.ID != selected[0].ID {
				filtered = append(filtered, m)
			}
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			left, right := countMatches(filtered[i].Genres, genres), countMatches(filtered[j].Genres, genres)
			if left != right {
				return left > right
			}
			return filtered[i].Rating > filtered[j].Rating
		})
		movies = filtered[:minInt(8, len(filtered))]
	}
	s.writeCompatMovies(w, movies)
}

func splitGenres(genres string) []string {
	values := []string{}
	for _, genre := range strings.Split(genres, ",") {
		if genre = strings.ToLower(strings.TrimSpace(genre)); genre != "" {
			values = append(values, genre)
		}
	}
	return values
}

func countMatches(genres string, tastes []string) int {
	count := 0
	genres = strings.ToLower(genres)
	for _, taste := range tastes {
		if strings.Contains(genres, taste) {
			count++
		}
	}
	return count
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *server) allMovies(suffix string) ([]movie, error) {
	rows, err := s.db.Query(`SELECT id,titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion FROM peliculas ` + suffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []movie{}
	for rows.Next() {
		var m movie
		if err := rows.Scan(&m.ID, &m.Title, &m.Synopsis, &m.Year, &m.Duration, &m.RatingLabel, &m.Genres, &m.Poster, &m.Price, &m.Rating, &m.Popularity, &m.State, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *server) compatMovieList(w http.ResponseWriter, _ *http.Request, query string) {
	s.compatMovieListArgs(w, nil, query)
}

func (s *server) compatMovieListArgs(w http.ResponseWriter, _ *http.Request, query string, args ...any) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("query compatibility movies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
		return
	}
	defer rows.Close()
	movies := []movie{}
	for rows.Next() {
		var m movie
		if err := rows.Scan(&m.ID, &m.Title, &m.Synopsis, &m.Year, &m.Duration, &m.RatingLabel, &m.Genres, &m.Poster, &m.Price, &m.Rating, &m.Popularity, &m.State, &m.CreatedAt); err != nil {
			fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
			return
		}
		movies = append(movies, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate compatibility movies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar las películas.")
		return
	}
	s.writeCompatMovies(w, movies)
}

func (s *server) writeCompatMovies(w http.ResponseWriter, movies []movie) {
	result := make([]map[string]any, 0, len(movies))
	for _, m := range movies {
		_, available, err := s.movieCounts(m.ID)
		if err != nil {
			log.Printf("count copies for compatibility movie %d: %v", m.ID, err)
			available = 0
		}
		result = append(result, map[string]any{
			"id": m.ID, "titulo": m.Title, "descripcion": m.Synopsis, "anio": m.Year,
			"duracion": m.Duration, "rating": m.Rating, "generos": m.Genres,
			"poster": m.Poster, "popularidad": m.Popularity, "precio_dia": m.Price,
			"disponibles": available,
		})
	}
	writeJSON(w, http.StatusOK, result)
}
