package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"
)

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	query := `SELECT id,nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion FROM usuarios WHERE 1=1`
	args := []any{}
	for _, field := range []struct{ query, column string }{{"rol", "rol"}, {"estado", "estado"}} {
		if value := r.URL.Query().Get(field.query); value != "" {
			query += ` AND ` + field.column + `=?`
			args = append(args, value)
		}
	}
	if term := strings.TrimSpace(r.URL.Query().Get("buscar")); term != "" {
		query += ` AND (lower(nombre) LIKE ? OR lower(correo) LIKE ?)`
		args = append(args, "%"+strings.ToLower(term)+"%", "%"+strings.ToLower(term)+"%")
	}
	query += ` ORDER BY fecha_creacion DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("list users: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los usuarios.")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			fail(w, http.StatusInternalServerError, "No se pudieron cargar los usuarios.")
			return
		}
		result = append(result, publicUser(u))
	}
	writeJSON(w, http.StatusOK, result)
}

func scanUser(row copyRow) (user, error) {
	var u user
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Phone, &u.Role, &u.State, &u.CreatedAt)
	return u, err
}

func (s *server) userByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	u, err := s.findUser(id)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Usuario no encontrado")
		return
	}
	if err != nil {
		log.Printf("load user: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo cargar el usuario.")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(u))
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	var input struct {
		Name     string  `json:"nombre"`
		Email    string  `json:"correo"`
		Password string  `json:"contrasena"`
		Phone    *string `json:"telefono"`
		Role     string  `json:"rol"`
	}
	if decodeBody(r, &input) != nil || len(strings.TrimSpace(input.Name)) < 2 || len(input.Password) < 6 || !validEmail(input.Email) {
		fail(w, http.StatusUnprocessableEntity, "Datos de usuario inválidos.")
		return
	}
	if input.Role == "" {
		input.Role = "CLIENTE"
	}
	if input.Role != "CLIENTE" && input.Role != "EMPLEADO" && input.Role != "ADMIN" {
		fail(w, http.StatusUnprocessableEntity, "Rol inválido.")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	now := time.Now().UTC().Format(dateLayout)
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		log.Printf("hash internal user password: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo asegurar la contraseña.")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO usuarios(nombre,correo,hash_contrasena,telefono,rol,estado,fecha_creacion,fecha_actualizacion) VALUES(?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(input.Name), input.Email, passwordHash, input.Phone, input.Role, "ACTIVO", now, now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			fail(w, http.StatusBadRequest, "Ya existe un usuario con este correo.")
			return
		}
		log.Printf("create user: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo crear el usuario.")
		return
	}
	u, err := s.userByEmail(input.Email)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el usuario creado.")
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(u))
}

func (s *server) setUserState(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.currentUser(w, r, "ADMIN")
	if !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var input struct {
		State string  `json:"estado"`
		Role  *string `json:"rol"`
	}
	if decodeBody(r, &input) != nil || (input.State != "ACTIVO" && input.State != "INACTIVO") {
		fail(w, http.StatusUnprocessableEntity, "Datos de estado inválidos.")
		return
	}
	if id == admin.ID && input.State != "ACTIVO" {
		fail(w, http.StatusBadRequest, "No puede desactivar su propia cuenta de administrador.")
		return
	}
	if input.Role != nil && *input.Role != "CLIENTE" && *input.Role != "EMPLEADO" && *input.Role != "ADMIN" {
		fail(w, http.StatusUnprocessableEntity, "Rol inválido.")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE id=?`, id).Scan(&exists); err != nil || exists == 0 {
		fail(w, http.StatusNotFound, "Usuario no encontrado")
		return
	}
	if input.Role == nil {
		_, err = s.db.Exec(`UPDATE usuarios SET estado=?,fecha_actualizacion=? WHERE id=?`, input.State, time.Now().UTC().Format(dateLayout), id)
	} else {
		_, err = s.db.Exec(`UPDATE usuarios SET estado=?,rol=?,fecha_actualizacion=? WHERE id=?`, input.State, *input.Role, time.Now().UTC().Format(dateLayout), id)
	}
	if err != nil {
		log.Printf("update user state: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo cambiar el estado del usuario.")
		return
	}
	u, err := s.findUser(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el usuario.")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(u))
}

func (s *server) reportSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	if err := s.updateOverdueRentals(); err != nil {
		log.Printf("update overdue rentals: %v", err)
	}
	counts := []string{
		`SELECT COUNT(*) FROM usuarios`,
		`SELECT COUNT(*) FROM usuarios WHERE rol='CLIENTE'`,
		`SELECT COUNT(*) FROM peliculas`,
		`SELECT COUNT(*) FROM copias`,
		`SELECT COUNT(*) FROM copias WHERE estado='DISPONIBLE'`,
		`SELECT COUNT(*) FROM copias WHERE estado='ALQUILADA'`,
		`SELECT COUNT(*) FROM alquileres WHERE estado='ACTIVO'`,
		`SELECT COUNT(*) FROM alquileres WHERE estado='VENCIDO'`,
		`SELECT COUNT(*) FROM alquileres`,
	}
	values := make([]int64, len(counts))
	for i, query := range counts {
		if err := s.db.QueryRow(query).Scan(&values[i]); err != nil {
			log.Printf("calculate report total: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudo generar el resumen.")
			return
		}
	}
	var revenue, lateRevenue float64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(importe),0) FROM pagos WHERE estado='COMPLETADO'`).Scan(&revenue); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo generar el resumen.")
		return
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(importe),0) FROM pagos WHERE estado='COMPLETADO' AND referencia LIKE 'DEV-%'`).Scan(&lateRevenue); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo generar el resumen.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_usuarios": values[0], "total_clientes": values[1], "total_peliculas": values[2],
		"total_copias": values[3], "copias_disponibles": values[4], "copias_alquiladas": values[5],
		"alquileres_activos": values[6], "alquileres_vencidos": values[7], "alquileres_totales": values[8],
		"ingresos_totales": roundMoney(revenue), "ingresos_recargos": roundMoney(lateRevenue),
	})
}

func (s *server) reportRevenue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	from, to := r.URL.Query().Get("fecha_inicio"), r.URL.Query().Get("fecha_fin")
	if (from != "" && !validDate(from)) || (to != "" && !validDate(to)) {
		fail(w, http.StatusUnprocessableEntity, "Las fechas deben tener formato YYYY-MM-DD.")
		return
	}
	query := `SELECT date(fecha),COUNT(DISTINCT alquiler_id),SUM(CASE WHEN referencia LIKE 'DEV-%' THEN 0 ELSE importe END),SUM(CASE WHEN referencia LIKE 'DEV-%' THEN importe ELSE 0 END),SUM(importe) FROM pagos WHERE estado='COMPLETADO'`
	args := []any{}
	if from != "" {
		query += ` AND date(fecha)>=?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND date(fecha)<=?`
		args = append(args, to)
	}
	query += ` GROUP BY date(fecha) ORDER BY date(fecha) DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("query revenue report: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de ingresos.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	total := float64(0)
	for rows.Next() {
		var date string
		var rentalCount int
		var rentalRevenue, lateRevenue, dailyTotal sql.NullFloat64
		if err := rows.Scan(&date, &rentalCount, &rentalRevenue, &lateRevenue, &dailyTotal); err != nil {
			fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de ingresos.")
			return
		}
		daily := dailyTotal.Float64
		total += daily
		items = append(items, map[string]any{
			"fecha": date, "alquileres_count": rentalCount,
			"ingresos_alquiler": roundMoney(rentalRevenue.Float64),
			"ingresos_recargo":  roundMoney(lateRevenue.Float64), "total": roundMoney(daily),
		})
	}
	if err := rows.Err(); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de ingresos.")
		return
	}
	topQuery := `SELECT p.id,p.titulo,p.poster_url,COUNT(a.id),COALESCE(SUM(pg.total),0) FROM peliculas p
		JOIN copias c ON c.pelicula_id=p.id JOIN alquileres a ON a.copia_id=c.id
		LEFT JOIN (SELECT alquiler_id,SUM(importe) total FROM pagos WHERE estado='COMPLETADO' GROUP BY alquiler_id) pg ON pg.alquiler_id=a.id WHERE 1=1`
	topArgs := []any{}
	if from != "" {
		topQuery += ` AND date(a.fecha_alquiler)>=?`
		topArgs = append(topArgs, from)
	}
	if to != "" {
		topQuery += ` AND date(a.fecha_alquiler)<=?`
		topArgs = append(topArgs, to)
	}
	topQuery += ` GROUP BY p.id ORDER BY COUNT(a.id) DESC LIMIT 10`
	topRows, err := s.db.Query(topQuery, topArgs...)
	if err != nil {
		log.Printf("query most rented movies: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo generar el ranking de películas.")
		return
	}
	defer topRows.Close()
	topMovies := []map[string]any{}
	for topRows.Next() {
		var id int64
		var title, poster string
		var rentalCount int
		var income float64
		if err := topRows.Scan(&id, &title, &poster, &rentalCount, &income); err != nil {
			fail(w, http.StatusInternalServerError, "No se pudo generar el ranking de películas.")
			return
		}
		topMovies = append(topMovies, map[string]any{"pelicula_id": id, "titulo": title,
			"poster_url": poster, "total_alquileres": rentalCount, "ingresos_generados": roundMoney(income)})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ingresos_totales": roundMoney(total), "desglose_por_fecha": items, "top_peliculas": topMovies,
	})
}

func (s *server) reportRentals(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN"); !ok {
		return
	}
	if err := s.updateOverdueRentals(); err != nil {
		log.Printf("update overdue rentals: %v", err)
	}
	query := `SELECT id FROM alquileres WHERE 1=1`
	args := []any{}
	if state := r.URL.Query().Get("estado"); state != "" {
		query += ` AND estado=?`
		args = append(args, state)
	}
	for _, field := range []struct{ key, op string }{{"fecha_inicio", ">="}, {"fecha_fin", "<="}} {
		if date := r.URL.Query().Get(field.key); date != "" {
			if !validDate(date) {
				fail(w, http.StatusUnprocessableEntity, "Las fechas deben tener formato YYYY-MM-DD.")
				return
			}
			query += ` AND date(fecha_alquiler) ` + field.op + ` ?`
			args = append(args, date)
		}
	}
	query += ` ORDER BY fecha_alquiler DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("query rentals report: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de alquileres.")
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de alquileres.")
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate rentals report: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de alquileres.")
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("close rental report rows: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de alquileres.")
		return
	}
	result := []map[string]any{}
	for _, id := range ids {
		item, err := s.rentalJSON(id)
		if err != nil {
			fail(w, http.StatusInternalServerError, "No se pudo generar el reporte de alquileres.")
			return
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

func validDate(value string) bool {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return false
	}
	return true
}
