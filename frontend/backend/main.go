package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const (
	defaultSecret = "cinema_secure_secret_key_2026_jwt_token_auth_dev_prod"
	dateLayout    = "2006-01-02 15:04:05.999999"
)

type server struct {
	db     *sql.DB
	secret []byte
	port   string
}

type user struct {
	ID        int64
	Name      string
	Email     string
	Password  string
	Phone     sql.NullString
	Role      string
	State     string
	CreatedAt string
}

func main() {
	dbPath := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbPath == "" {
		dbPath = "file:cinema.db"
	} else {
		dbPath = strings.TrimPrefix(dbPath, "sqlite:///")
		if !strings.HasPrefix(dbPath, "file:") {
			dbPath = "file:" + dbPath
		}
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	app := &server{
		db:     db,
		secret: []byte(envOr("SECRET_KEY", defaultSecret)),
		port:   envOr("PORT", "8001"),
	}
	if err := app.initialize(); err != nil {
		log.Fatal(err)
	}
	log.Printf("CINEMA API listening on :%s", app.port)
	log.Fatal(http.ListenAndServe(":"+app.port, app.routes()))
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (s *server) initialize() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS usuarios (id INTEGER PRIMARY KEY AUTOINCREMENT, nombre VARCHAR(120) NOT NULL, correo VARCHAR(120) NOT NULL UNIQUE, hash_contrasena VARCHAR(255) NOT NULL, telefono VARCHAR(30), rol VARCHAR(8) NOT NULL, estado VARCHAR(8) NOT NULL, fecha_creacion DATETIME NOT NULL, fecha_actualizacion DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS peliculas (id INTEGER PRIMARY KEY AUTOINCREMENT, titulo VARCHAR(180) NOT NULL, sinopsis TEXT NOT NULL, anio INTEGER NOT NULL, duracion INTEGER NOT NULL, clasificacion VARCHAR(20) NOT NULL, generos VARCHAR(200) NOT NULL, poster_url VARCHAR(500) NOT NULL, precio_dia FLOAT NOT NULL, rating FLOAT NOT NULL, popularidad FLOAT NOT NULL, estado VARCHAR(8) NOT NULL, fecha_creacion DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS copias (id INTEGER PRIMARY KEY AUTOINCREMENT, pelicula_id INTEGER NOT NULL, codigo_inventario VARCHAR(50) NOT NULL UNIQUE, estado VARCHAR(13) NOT NULL, ubicacion VARCHAR(80), observaciones VARCHAR(255), FOREIGN KEY(pelicula_id) REFERENCES peliculas(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS alquileres (id INTEGER PRIMARY KEY AUTOINCREMENT, cliente_id INTEGER NOT NULL, copia_id INTEGER NOT NULL, fecha_alquiler DATETIME NOT NULL, fecha_prevista_devolucion DATETIME NOT NULL, fecha_real_devolucion DATETIME, precio_alquiler FLOAT NOT NULL, recargo FLOAT NOT NULL, estado VARCHAR(9) NOT NULL, dias_alquiler INTEGER NOT NULL, observaciones TEXT, FOREIGN KEY(cliente_id) REFERENCES usuarios(id), FOREIGN KEY(copia_id) REFERENCES copias(id))`,
		`CREATE TABLE IF NOT EXISTS devoluciones (id INTEGER PRIMARY KEY AUTOINCREMENT, alquiler_id INTEGER NOT NULL UNIQUE, fecha_devolucion DATETIME NOT NULL, dias_retraso INTEGER NOT NULL, recargo_calculado FLOAT NOT NULL, observaciones TEXT, procesado_por_id INTEGER, FOREIGN KEY(alquiler_id) REFERENCES alquileres(id), FOREIGN KEY(procesado_por_id) REFERENCES usuarios(id))`,
		`CREATE TABLE IF NOT EXISTS pagos (id INTEGER PRIMARY KEY AUTOINCREMENT, alquiler_id INTEGER NOT NULL, importe FLOAT NOT NULL, metodo VARCHAR(13) NOT NULL, fecha DATETIME NOT NULL, estado VARCHAR(11) NOT NULL, referencia VARCHAR(100), FOREIGN KEY(alquiler_id) REFERENCES alquileres(id))`,
		`CREATE TABLE IF NOT EXISTS configuracion_sistema (id INTEGER PRIMARY KEY AUTOINCREMENT, clave VARCHAR(50) NOT NULL UNIQUE, valor VARCHAR(100) NOT NULL, descripcion VARCHAR(255))`,
		`CREATE INDEX IF NOT EXISTS ix_copias_pelicula_id ON copias(pelicula_id)`,
		`CREATE INDEX IF NOT EXISTS ix_alquileres_cliente_id ON alquileres(cliente_id)`,
		`CREATE INDEX IF NOT EXISTS ix_pagos_alquiler_id ON pagos(alquiler_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize database: %w", err)
		}
	}
	return s.seed()
}

func (s *server) seed() error {
	now := time.Now().UTC().Format(dateLayout)
	configs, err := s.db.Query(`SELECT clave FROM configuracion_sistema LIMIT 1`)
	if err != nil {
		return err
	}
	hasConfig := configs.Next()
	configs.Close()
	if !hasConfig {
		for _, row := range [][3]string{
			{"TARIFA_DIARIA_BASE", "2.50", "Tarifa diaria base por defecto"},
			{"RECARGO_DIARIO_MORA", "1.50", "Recargo por cada día de retraso en devolución"},
			{"DIAS_ALQUILER_DEFECTO", "3", "Días estándar de alquiler"},
		} {
			if _, err := s.db.Exec(`INSERT INTO configuracion_sistema(clave,valor,descripcion) VALUES(?,?,?)`, row[0], row[1], row[2]); err != nil {
				return err
			}
		}
	}
	count := int64(0)
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usuarios`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		accounts := []struct{ name, email, password, phone, role string }{
			{"Carlos Administrador", "admin@cinema.com", "admin123", "+51 600 111 222", "ADMIN"},
			{"Lucía Empleada", "empleado@cinema.com", "empleado123", "+51 600 333 444", "EMPLEADO"},
			{"Tomás Cliente", "cliente@cinema.com", "cliente123", "+51 600 555 666", "CLIENTE"},
			{"Sofía Mendoza", "cliente2@gmail.com", "cliente123", "+51 600 777 888", "CLIENTE"},
		}
		for _, account := range accounts {
			hash, err := bcrypt.GenerateFromPassword([]byte(account.password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			if _, err := s.db.Exec(`INSERT INTO usuarios(nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion,fecha_actualizacion) VALUES(?,?,?,?,?,?,?,?)`,
				account.name, account.email, string(hash), account.phone, account.role, "ACTIVO", now, now); err != nil {
				return err
			}
		}
	}
	var movieCount int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM peliculas`).Scan(&movieCount); err != nil {
		return err
	}
	if movieCount == 0 {
		catalog := []struct {
			title, synopsis           string
			year, duration            int
			label, genres             string
			poster                    string
			price, rating, popularity float64
		}{
			{"Inception", "Un ladrón que roba secretos mediante sueños recibe la misión de plantar una idea.", 2010, 148, "PG-13", "Action, Science Fiction, Adventure", "https://images.unsplash.com/photo-1536440136628-849c177e76a1?w=800", 2.5, 8.8, 98.5},
			{"Interstellar", "Un equipo viaja por un agujero de gusano para buscar un futuro para la humanidad.", 2014, 169, "PG-13", "Science Fiction, Adventure, Drama", "https://images.unsplash.com/photo-1451187580459-43490279c0fa?w=800", 2.8, 8.7, 96},
			{"The Dark Knight", "Batman se enfrenta al Joker y a una de las pruebas más difíciles de Gotham.", 2008, 152, "PG-13", "Action, Crime, Drama", "https://images.unsplash.com/photo-1509198397868-475647b2a1e5?w=800", 2.5, 9.0, 99},
			{"Dune: Part Two", "Paul Atreides se une a Chani y los Fremen para enfrentarse a sus conspiradores.", 2024, 166, "PG-13", "Science Fiction, Adventure, Action", "https://images.unsplash.com/photo-1518709268805-4e9042af9f23?w=800", 3.5, 8.6, 99.5},
			{"Pulp Fiction", "Las vidas de dos sicarios, un boxeador y varios criminales se entrelazan.", 1994, 154, "R", "Crime, Drama, Thriller", "https://images.unsplash.com/photo-1594909122845-11baa439b7bf?w=800", 2.0, 8.9, 94},
			{"Spider-Man: Across the Spider-Verse", "Miles Morales conoce a un equipo de Spider-People del multiverso.", 2023, 140, "PG", "Action, Adventure, Animation", "https://images.unsplash.com/photo-1635805737707-575885ab0820?w=800", 3.0, 8.7, 97},
			{"The Shining", "Una presencia siniestra transforma la estancia de una familia en un hotel aislado.", 1980, 146, "R", "Horror, Drama, Mystery", "https://images.unsplash.com/photo-1509281373149-e957c6296406?w=800", 2.0, 8.4, 89},
			{"Superbad", "Dos amigos de secundaria intentan organizar una fiesta antes de graduarse.", 2007, 113, "R", "Comedy", "https://images.unsplash.com/photo-1489599849927-2ee91cede3ba?w=800", 2.0, 7.6, 85},
			{"Alien: Romulus", "Un grupo de jóvenes colonizadores se enfrenta a una forma de vida aterradora.", 2024, 119, "R", "Horror, Science Fiction", "https://images.unsplash.com/photo-1579783902614-a3fb3927b675?w=800", 3.5, 7.3, 92},
			{"Gladiator II", "Lucio entra al Coliseo tras la conquista de su hogar por emperadores tiránicos.", 2024, 148, "R", "Action, Adventure, Drama", "https://images.unsplash.com/photo-1440404653325-ab127d49abc1?w=800", 3.5, 7.5, 93},
		}
		for _, item := range catalog {
			result, err := s.db.Exec(`INSERT INTO peliculas(titulo,sinopsis,anio,duracion,clasificacion,generos,poster_url,precio_dia,rating,popularidad,estado,fecha_creacion) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				item.title, item.synopsis, item.year, item.duration, item.label, item.genres, item.poster, item.price, item.rating, item.popularity, "ACTIVO", now)
			if err != nil {
				return err
			}
			movieID, err := result.LastInsertId()
			if err != nil {
				return err
			}
			for copyNumber := 1; copyNumber <= 3; copyNumber++ {
				code := fmt.Sprintf("CP-%04d-%02d", movieID, copyNumber)
				if _, err := s.db.Exec(`INSERT INTO copias(pelicula_id,codigo_inventario,estado,ubicacion) VALUES(?,?,?,?)`,
					movieID, code, "DISPONIBLE", fmt.Sprintf("Pasillo A - Estante %d", movieID)); err != nil {
					return err
				}
			}
		}
	}
	var rentalCount int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM alquileres`).Scan(&rentalCount); err != nil {
		return err
	}
	if rentalCount == 0 {
		var clientID, copyID int64
		var price float64
		err := s.db.QueryRow(`SELECT u.id,c.id,p.precio_dia FROM usuarios u CROSS JOIN copias c JOIN peliculas p ON p.id=c.pelicula_id
			WHERE u.correo='cliente@cinema.com' AND c.estado='DISPONIBLE' ORDER BY c.id LIMIT 1`).Scan(&clientID, &copyID, &price)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			start := time.Now().UTC().AddDate(0, 0, -1)
			due := time.Now().UTC().AddDate(0, 0, 3)
			amount := roundMoney(price * 4)
			result, err := s.db.Exec(`INSERT INTO alquileres(cliente_id,copia_id,fecha_alquiler,fecha_prevista_devolucion,precio_alquiler,recargo,estado,dias_alquiler,observaciones)
				VALUES(?,?,?,?,?,0,'ACTIVO',4,?)`, clientID, copyID, start.Format(dateLayout), due.Format(dateLayout), amount, "Alquiler inicial de prueba")
			if err != nil {
				return err
			}
			rentalID, err := result.LastInsertId()
			if err != nil {
				return err
			}
			if _, err := s.db.Exec(`UPDATE copias SET estado='ALQUILADA' WHERE id=?`, copyID); err != nil {
				return err
			}
			if _, err := s.db.Exec(`INSERT INTO pagos(alquiler_id,importe,metodo,fecha,estado,referencia) VALUES(?,?,?,?,?,?)`,
				rentalID, amount, "TARJETA", start.Format(dateLayout), "COMPLETADO", fmt.Sprintf("ALQ-%05d-INIT", rentalID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /docs", s.docs)
	mux.HandleFunc("GET /redoc", s.docs)
	mux.HandleFunc("GET /openapi.json", s.openAPI)

	mux.HandleFunc("POST /api/auth/registro", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("PUT /api/auth/perfil", s.updateProfile)
	mux.HandleFunc("PUT /api/auth/cambiar-contrasena", s.changePassword)

	mux.HandleFunc("GET /api/peliculas", s.listMovies)
	mux.HandleFunc("POST /api/peliculas", s.createMovie)
	mux.HandleFunc("GET /api/peliculas/{id}", s.movieByID)
	mux.HandleFunc("PUT /api/peliculas/{id}", s.updateMovie)
	mux.HandleFunc("PATCH /api/peliculas/{id}/estado", s.setMovieState)
	mux.HandleFunc("GET /api/peliculas/{id}/disponibilidad", s.movieAvailability)
	mux.HandleFunc("GET /peliculas", s.compatMovies)
	mux.HandleFunc("GET /tendencias", s.compatTrends)
	mux.HandleFunc("GET /mejor-calificadas", s.compatTopRated)
	mux.HandleFunc("GET /genero/{genero}", s.compatGenre)
	mux.HandleFunc("GET /buscar", s.compatSearch)
	mux.HandleFunc("POST /recomendar", s.compatRecommend)
	mux.HandleFunc("POST /similar", s.compatSimilar)

	mux.HandleFunc("GET /api/copias", s.listCopies)
	mux.HandleFunc("POST /api/copias", s.createCopy)
	mux.HandleFunc("PUT /api/copias/{id}", s.updateCopy)
	mux.HandleFunc("POST /api/alquileres", s.createRental)
	mux.HandleFunc("GET /api/alquileres", s.listRentals)
	mux.HandleFunc("GET /api/alquileres/mis-alquileres", s.myRentals)
	mux.HandleFunc("GET /api/alquileres/{id}", s.rentalByID)
	mux.HandleFunc("POST /api/alquileres/{id}/devolucion", s.returnRental)
	mux.HandleFunc("GET /api/alquileres/{id}/pagos", s.rentalPayments)
	mux.HandleFunc("GET /api/pagos", s.listPayments)
	mux.HandleFunc("POST /api/pagos", s.createPayment)
	mux.HandleFunc("GET /api/usuarios", s.listUsers)
	mux.HandleFunc("POST /api/usuarios", s.createUser)
	mux.HandleFunc("GET /api/usuarios/{id}", s.userByID)
	mux.HandleFunc("PATCH /api/usuarios/{id}/estado", s.setUserState)
	mux.HandleFunc("GET /api/admin/reportes/resumen", s.reportSummary)
	mux.HandleFunc("GET /api/admin/reportes/ingresos", s.reportRevenue)
	mux.HandleFunc("GET /api/admin/reportes/alquileres", s.reportRentals)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "online", "app": "CINEMA Alquiler API", "port": s.port})
}

func (s *server) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="es"><meta charset="utf-8"><title>CINEMA API</title><body><h1>CINEMA API</h1><p>Documentación de la API REST.</p><p><a href="/openapi.json">Especificación OpenAPI</a></p></body></html>`))
}

func (s *server) openAPI(w http.ResponseWriter, _ *http.Request) {
	paths := map[string]any{}
	endpoints := []struct{ path, method, summary string }{
		{"/api/health", "get", "Estado del servicio"},
		{"/api/auth/registro", "post", "Registrar cliente"},
		{"/api/auth/login", "post", "Iniciar sesión"},
		{"/api/auth/me", "get", "Consultar perfil"},
		{"/api/auth/perfil", "put", "Actualizar perfil"},
		{"/api/auth/cambiar-contrasena", "put", "Cambiar contraseña"},
		{"/api/peliculas", "get", "Listar y filtrar películas"},
		{"/api/peliculas", "post", "Crear película"},
		{"/api/peliculas/{id}", "get", "Consultar película"},
		{"/api/peliculas/{id}", "put", "Actualizar película"},
		{"/api/peliculas/{id}/estado", "patch", "Cambiar estado de película"},
		{"/api/peliculas/{id}/disponibilidad", "get", "Consultar copias disponibles"},
		{"/api/copias", "get", "Listar copias"},
		{"/api/copias", "post", "Crear copia"},
		{"/api/copias/{id}", "put", "Actualizar copia"},
		{"/api/alquileres", "get", "Listar alquileres"},
		{"/api/alquileres", "post", "Crear alquiler"},
		{"/api/alquileres/mis-alquileres", "get", "Consultar mis alquileres"},
		{"/api/alquileres/{id}", "get", "Consultar alquiler"},
		{"/api/alquileres/{id}/devolucion", "post", "Registrar devolución"},
		{"/api/alquileres/{id}/pagos", "get", "Consultar pagos del alquiler"},
		{"/api/pagos", "get", "Listar pagos"},
		{"/api/pagos", "post", "Registrar pago"},
		{"/api/usuarios", "get", "Listar usuarios"},
		{"/api/usuarios", "post", "Crear usuario"},
		{"/api/usuarios/{id}", "get", "Consultar usuario"},
		{"/api/usuarios/{id}/estado", "patch", "Cambiar estado de usuario"},
		{"/api/admin/reportes/resumen", "get", "Consultar resumen administrativo"},
		{"/api/admin/reportes/ingresos", "get", "Consultar reporte de ingresos"},
		{"/api/admin/reportes/alquileres", "get", "Consultar reporte de alquileres"},
		{"/peliculas", "get", "Catálogo compatible CineMatch"},
		{"/tendencias", "get", "Películas en tendencia"},
		{"/mejor-calificadas", "get", "Películas mejor calificadas"},
		{"/genero/{genero}", "get", "Buscar películas por género"},
		{"/buscar", "get", "Buscar películas"},
		{"/recomendar", "post", "Recomendar películas"},
		{"/similar", "post", "Buscar películas similares"},
	}
	for _, endpoint := range endpoints {
		methods, ok := paths[endpoint.path].(map[string]any)
		if !ok {
			methods = map[string]any{}
			paths[endpoint.path] = methods
		}
		methods[endpoint.method] = map[string]any{
			"summary":   endpoint.summary,
			"responses": map[string]any{"200": map[string]string{"description": "Respuesta JSON"}},
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]string{"title": "CINEMA Alquiler API", "version": "1.0.0"},
		"paths":   paths,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func fail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func decodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("request body too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("request contains trailing data")
	}
	return nil
}

func parseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func (s *server) token(subject int64, role string) (string, error) {
	now := time.Now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(map[string]any{
		"sub": strconv.FormatInt(subject, 10), "role": role,
		"iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *server) parseToken(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return 0, errors.New("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("invalid payload")
	}
	var claims struct {
		Subject string `json:"sub"`
		Expiry  int64  `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Subject == "" || time.Now().Unix() >= claims.Expiry {
		return 0, errors.New("expired or malformed token")
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

func (s *server) findUser(id int64) (user, error) {
	var u user
	err := s.db.QueryRow(`SELECT id,nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion FROM usuarios WHERE id=?`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Phone, &u.Role, &u.State, &u.CreatedAt)
	return u, err
}

func (s *server) currentUser(w http.ResponseWriter, r *http.Request, roles ...string) (user, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		fail(w, http.StatusUnauthorized, "Autenticación requerida. Token no proporcionado.")
		return user{}, false
	}
	id, err := s.parseToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		fail(w, http.StatusUnauthorized, "Token de acceso inválido o expirado.")
		return user{}, false
	}
	u, err := s.findUser(id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "Usuario no encontrado en el sistema.")
		return user{}, false
	}
	if err != nil {
		log.Printf("load authenticated user: %v", err)
		fail(w, http.StatusInternalServerError, "Error interno del servidor.")
		return user{}, false
	}
	if u.State != "ACTIVO" {
		fail(w, http.StatusForbidden, "La cuenta de usuario se encuentra inactiva.")
		return user{}, false
	}
	if len(roles) > 0 {
		for _, role := range roles {
			if u.Role == role {
				return u, true
			}
		}
		fail(w, http.StatusForbidden, "Permiso denegado.")
		return user{}, false
	}
	return u, true
}

func publicUser(u user) map[string]any {
	var phone any
	if u.Phone.Valid {
		phone = u.Phone.String
	}
	return map[string]any{
		"id": u.ID, "nombre": u.Name, "correo": u.Email, "telefono": phone,
		"rol": u.Role, "estado": u.State, "fecha_creacion": parseTime(u.CreatedAt),
	}
}

func parseTime(value string) time.Time {
	for _, layout := range []string{dateLayout, "2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string  `json:"nombre"`
		Email    string  `json:"correo"`
		Password string  `json:"contrasena"`
		Phone    *string `json:"telefono"`
		Role     string  `json:"rol"`
	}
	if decodeBody(r, &input) != nil || len(strings.TrimSpace(input.Name)) < 2 || len(input.Password) < 6 || !validEmail(input.Email) {
		fail(w, http.StatusUnprocessableEntity, "Datos de registro inválidos.")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		log.Printf("hash registered password: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo asegurar la contraseña.")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO usuarios(nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion,fecha_actualizacion) VALUES(?,?,?,?,?,?,?,?)`,
		input.Name, input.Email, passwordHash, input.Phone, "CLIENTE", "ACTIVO", time.Now().UTC().Format(dateLayout), time.Now().UTC().Format(dateLayout)); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			fail(w, http.StatusBadRequest, "Ya existe un usuario registrado con este correo electrónico.")
			return
		}
		log.Printf("register user: %v", err)
		fail(w, http.StatusInternalServerError, "Error interno del servidor.")
		return
	}
	u, err := s.userByEmail(input.Email)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el usuario creado.")
		return
	}
	s.sendToken(w, u, http.StatusCreated)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"correo"`
		Password string `json:"contrasena"`
	}
	if decodeBody(r, &input) != nil || !validEmail(input.Email) || len(input.Password) < 4 {
		fail(w, http.StatusUnprocessableEntity, "Credenciales inválidas.")
		return
	}
	u, err := s.userByEmail(strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(input.Password)) != nil {
		fail(w, http.StatusUnauthorized, "Correo electrónico o contraseña incorrectos.")
		return
	}
	if u.State != "ACTIVO" {
		fail(w, http.StatusForbidden, "Su cuenta de usuario ha sido desactivada. Comuníquese con administración.")
		return
	}
	s.sendToken(w, u, http.StatusOK)
}

func (s *server) userByEmail(email string) (user, error) {
	var u user
	err := s.db.QueryRow(`SELECT id,nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion FROM usuarios WHERE lower(correo)=?`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Phone, &u.Role, &u.State, &u.CreatedAt)
	return u, err
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *server) sendToken(w http.ResponseWriter, u user, status int) {
	token, err := s.token(u.ID, u.Role)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo generar el token.")
		return
	}
	writeJSON(w, status, map[string]any{"access_token": token, "token_type": "bearer", "usuario": publicUser(u)})
}

func validEmail(email string) bool {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-3 && strings.Contains(email[at+1:], ".")
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if ok {
		writeJSON(w, http.StatusOK, publicUser(u))
	}
}

func (s *server) updateProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Name  *string `json:"nombre"`
		Phone *string `json:"telefono"`
	}
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de perfil inválidos.")
		return
	}
	if input.Name != nil {
		u.Name = strings.TrimSpace(*input.Name)
	}
	if input.Phone != nil {
		u.Phone = sql.NullString{String: strings.TrimSpace(*input.Phone), Valid: true}
	}
	if _, err := s.db.Exec(`UPDATE usuarios SET nombre=?,telefono=?,fecha_actualizacion=? WHERE id=?`, u.Name, nullValue(u.Phone), time.Now().UTC().Format(dateLayout), u.ID); err != nil {
		log.Printf("update user profile: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo actualizar el perfil.")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(u))
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Current string `json:"contrasena_actual"`
		New     string `json:"nueva_contrasena"`
	}
	if decodeBody(r, &input) != nil || len(input.New) < 6 {
		fail(w, http.StatusUnprocessableEntity, "Datos de contraseña inválidos.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(input.Current)) != nil {
		fail(w, http.StatusBadRequest, "La contraseña actual no es correcta.")
		return
	}
	passwordHash, err := hashPassword(input.New)
	if err != nil {
		log.Printf("hash changed password: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo asegurar la contraseña.")
		return
	}
	if _, err := s.db.Exec(`UPDATE usuarios SET hash_contrasena=?,fecha_actualizacion=? WHERE id=?`, passwordHash, time.Now().UTC().Format(dateLayout), u.ID); err != nil {
		log.Printf("change password: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo actualizar la contraseña.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mensaje": "Contraseña actualizada exitosamente."})
}

func nullValue(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}
