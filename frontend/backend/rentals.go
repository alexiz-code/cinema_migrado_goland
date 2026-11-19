package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

func (s *server) lateFeePerDay() float64 {
	var value string
	if err := s.db.QueryRow(`SELECT valor FROM configuracion_sistema WHERE clave='RECARGO_DIARIO_MORA'`).Scan(&value); err == nil {
		if rate, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
			return rate
		}
	}
	return 1.5
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func (s *server) updateOverdueRentals() error {
	_, err := s.db.Exec(`UPDATE alquileres SET estado='VENCIDO' WHERE estado='ACTIVO' AND fecha_prevista_devolucion < ?`, time.Now().UTC().Format(dateLayout))
	return err
}

func (s *server) createRental(w http.ResponseWriter, r *http.Request) {
	current, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	var input struct {
		MovieID  int64  `json:"pelicula_id"`
		CopyID   *int64 `json:"copia_id"`
		Days     int    `json:"dias_alquiler"`
		Method   string `json:"metodo_pago"`
		ClientID *int64 `json:"cliente_id"`
	}
	if decodeBody(r, &input) != nil || input.MovieID < 1 {
		fail(w, http.StatusUnprocessableEntity, "Datos de alquiler inválidos.")
		return
	}
	if input.Days == 0 {
		input.Days = 3
	}
	if input.Method == "" {
		input.Method = "TARJETA"
	}
	if input.Days < 1 || input.Days > 30 || !validPaymentMethod(input.Method) {
		fail(w, http.StatusUnprocessableEntity, "Datos de alquiler inválidos.")
		return
	}
	clientID := current.ID
	if input.ClientID != nil && *input.ClientID != current.ID {
		if current.Role != "ADMIN" && current.Role != "EMPLEADO" {
			fail(w, http.StatusForbidden, "Solo administradores o empleados pueden registrar alquileres a nombre de terceros.")
			return
		}
		clientID = *input.ClientID
	}
	result, err := s.createRentalTransaction(clientID, input.MovieID, input.CopyID, input.Days, input.Method)
	if err != nil {
		var httpErr *apiError
		if errors.As(err, &httpErr) {
			fail(w, httpErr.status, httpErr.detail)
		} else {
			log.Printf("create rental: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudo crear el alquiler.")
		}
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

type apiError struct {
	status int
	detail string
}

func (e *apiError) Error() string { return e.detail }

func (s *server) createRentalTransaction(clientID, movieID int64, requestedCopy *int64, days int, method string) (map[string]any, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var clientExists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE id=?`, clientID).Scan(&clientExists); err != nil {
		return nil, err
	}
	if clientExists == 0 {
		return nil, &apiError{http.StatusNotFound, "Cliente no encontrado"}
	}
	var title, state string
	var dailyPrice float64
	if err := tx.QueryRow(`SELECT titulo,estado,precio_dia FROM peliculas WHERE id=?`, movieID).Scan(&title, &state, &dailyPrice); err != nil {
		if err == sql.ErrNoRows {
			return nil, &apiError{http.StatusNotFound, "Película no encontrada"}
		}
		return nil, err
	}
	if state != "ACTIVO" {
		return nil, &apiError{http.StatusBadRequest, "La película no está disponible para alquiler (inactiva)"}
	}
	var copyID int64
	var code, copyState string
	if requestedCopy != nil && *requestedCopy > 0 {
		err = tx.QueryRow(`SELECT id,codigo_inventario,estado FROM copias WHERE id=? AND pelicula_id=?`, *requestedCopy, movieID).Scan(&copyID, &code, &copyState)
		if err == sql.ErrNoRows {
			return nil, &apiError{http.StatusNotFound, "Copia física no encontrada"}
		}
		if err != nil {
			return nil, err
		}
		if copyState != "DISPONIBLE" {
			return nil, &apiError{http.StatusBadRequest, fmt.Sprintf("La copia seleccionada (%s) no está disponible (Estado actual: %s)", code, copyState)}
		}
	} else {
		err = tx.QueryRow(`SELECT id,codigo_inventario FROM copias WHERE pelicula_id=? AND estado='DISPONIBLE' ORDER BY id LIMIT 1`, movieID).Scan(&copyID, &code)
		if err == sql.ErrNoRows {
			return nil, &apiError{http.StatusBadRequest, fmt.Sprintf("No hay copias físicas disponibles para la película '%s'", title)}
		}
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	due := now.AddDate(0, 0, days)
	price := roundMoney(dailyPrice * float64(days))
	update, err := tx.Exec(`UPDATE copias SET estado='ALQUILADA' WHERE id=? AND estado='DISPONIBLE'`, copyID)
	if err != nil {
		return nil, err
	}
	changed, err := update.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		return nil, &apiError{http.StatusBadRequest, fmt.Sprintf("La copia seleccionada (%s) no está disponible (Estado actual: ALQUILADA)", code)}
	}
	result, err := tx.Exec(`INSERT INTO alquileres(cliente_id,copia_id,fecha_alquiler,fecha_prevista_devolucion,precio_alquiler,recargo,estado,dias_alquiler) VALUES(?,?,?,?,?,0,'ACTIVO',?)`,
		clientID, copyID, now.Format(dateLayout), due.Format(dateLayout), price, days)
	if err != nil {
		return nil, err
	}
	rentalID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO pagos(alquiler_id,importe,metodo,fecha,estado,referencia) VALUES(?,?,?,?,?,?)`,
		rentalID, price, method, now.Format(dateLayout), "PENDIENTE", fmt.Sprintf("ALQ-%05d-INIT", rentalID)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.rentalJSON(rentalID)
}

func (s *server) rentalJSON(id int64) (map[string]any, error) {
	var item map[string]any
	var rentalID, clientID, copyID int64
	var clientName, clientEmail, code, title, poster, state string
	var rentedAt, dueAt string
	var returnedAt sql.NullString
	var days int
	var price, surcharge float64
	err := s.db.QueryRow(`SELECT a.id,a.cliente_id,u.nombre,u.correo,a.copia_id,c.codigo_inventario,p.titulo,p.poster_url,a.fecha_alquiler,a.fecha_prevista_devolucion,a.fecha_real_devolucion,a.dias_alquiler,a.precio_alquiler,a.recargo,a.estado
		FROM alquileres a JOIN usuarios u ON u.id=a.cliente_id JOIN copias c ON c.id=a.copia_id JOIN peliculas p ON p.id=c.pelicula_id WHERE a.id=?`, id).
		Scan(&rentalID, &clientID, &clientName, &clientEmail, &copyID, &code, &title, &poster, &rentedAt, &dueAt, &returnedAt, &days, &price, &surcharge, &state)
	if err != nil {
		return nil, err
	}
	var movieID int64
	if err := s.db.QueryRow(`SELECT p.id FROM alquileres a JOIN copias c ON c.id=a.copia_id JOIN peliculas p ON p.id=c.pelicula_id WHERE a.id=?`, id).Scan(&movieID); err != nil {
		return nil, err
	}
	var paid float64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(importe),0) FROM pagos WHERE alquiler_id=? AND estado='COMPLETADO'`, id).Scan(&paid); err != nil {
		return nil, err
	}
	due := parseTime(dueAt)
	daysLate := 0
	estimatedFee := float64(0)
	now := time.Now().UTC()
	if (state == "ACTIVO" || state == "VENCIDO") && now.After(due) {
		daysLate = int(now.Truncate(24*time.Hour).Sub(due.Truncate(24*time.Hour)).Hours() / 24)
		if daysLate < 0 {
			daysLate = 0
		}
		estimatedFee = roundMoney(float64(daysLate) * s.lateFeePerDay())
	} else if state == "DEVUELTO" {
		estimatedFee = surcharge
	}
	item = map[string]any{
		"id": rentalID, "cliente_id": clientID, "cliente_nombre": clientName, "cliente_correo": clientEmail,
		"copia_id": copyID, "codigo_copia": code, "pelicula_id": movieID, "pelicula_titulo": title,
		"pelicula_poster": poster, "fecha_alquiler": parseTime(rentedAt), "fecha_prevista_devolucion": due,
		"fecha_real_devolucion": nullableTime(returnedAt), "dias_alquiler": days, "precio_alquiler": price,
		"recargo": surcharge, "total_pagado": roundMoney(paid), "estado": state,
		"dias_retraso_actual": daysLate, "recargo_estimado": estimatedFee,
	}
	return item, nil
}

func nullableTime(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return parseTime(value.String)
}

func (s *server) listRentals(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	s.rentalList(w, r, u, false)
}

func (s *server) myRentals(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	s.rentalList(w, r, u, true)
}

func (s *server) rentalList(w http.ResponseWriter, r *http.Request, u user, mine bool) {
	if err := s.updateOverdueRentals(); err != nil {
		log.Printf("update overdue rentals: %v", err)
	}
	query := `SELECT id FROM alquileres WHERE 1=1`
	args := []any{}
	if mine || u.Role == "CLIENTE" {
		query += ` AND cliente_id=?`
		args = append(args, u.ID)
	}
	if !mine {
		if raw := r.URL.Query().Get("cliente_id"); raw != "" && u.Role != "CLIENTE" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				fail(w, http.StatusUnprocessableEntity, "cliente_id inválido.")
				return
			}
			query += ` AND cliente_id=?`
			args = append(args, id)
		}
	}
	if state := r.URL.Query().Get("estado"); state != "" {
		query += ` AND estado=?`
		args = append(args, state)
	}
	query += ` ORDER BY fecha_alquiler DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("list rentals: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los alquileres.")
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			fail(w, http.StatusInternalServerError, "No se pudieron cargar los alquileres.")
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		log.Printf("iterate rentals: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los alquileres.")
		return
	}
	if err := rows.Close(); err != nil {
		log.Printf("close rental rows: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los alquileres.")
		return
	}
	result := []map[string]any{}
	for _, id := range ids {
		item, err := s.rentalJSON(id)
		if err != nil {
			log.Printf("build rental response: %v", err)
			fail(w, http.StatusInternalServerError, "No se pudo construir la respuesta.")
			return
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) rentalByID(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	if err := s.updateOverdueRentals(); err != nil {
		log.Printf("update overdue rentals: %v", err)
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var clientID int64
	if err := s.db.QueryRow(`SELECT cliente_id FROM alquileres WHERE id=?`, id).Scan(&clientID); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Alquiler no encontrado")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el alquiler.")
		return
	}
	if u.Role == "CLIENTE" && clientID != u.ID {
		fail(w, http.StatusForbidden, "No tiene permiso para ver este alquiler.")
		return
	}
	item, err := s.rentalJSON(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el alquiler.")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) returnRental(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO")
	if !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var input struct {
		Observations *string  `json:"observaciones"`
		ManualFee    *float64 `json:"recargo_manual"`
		ChargeNow    bool     `json:"cobrar_recargo_ahora"`
		Method       *string  `json:"metodo_pago_recargo"`
	}
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de devolución inválidos.")
		return
	}
	var clientID, copyID int64
	var due, state string
	if err := s.db.QueryRow(`SELECT cliente_id,copia_id,fecha_prevista_devolucion,estado FROM alquileres WHERE id=?`, id).
		Scan(&clientID, &copyID, &due, &state); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Alquiler no encontrado")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo procesar la devolución.")
		return
	}
	if state == "DEVUELTO" {
		fail(w, http.StatusBadRequest, "Este alquiler ya fue devuelto previamente")
		return
	}
	now := time.Now().UTC()
	dueTime := parseTime(due)
	lateDays := 0
	if now.After(dueTime) {
		lateDays = int(now.Truncate(24*time.Hour).Sub(dueTime.Truncate(24*time.Hour)).Hours() / 24)
		if lateDays < 0 {
			lateDays = 0
		}
	}
	fee := roundMoney(float64(lateDays) * s.lateFeePerDay())
	if input.ManualFee != nil {
		fee = math.Max(0, *input.ManualFee)
	}
	tx, err := s.db.Begin()
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo procesar la devolución.")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE alquileres SET fecha_real_devolucion=?,estado='DEVUELTO',recargo=? WHERE id=? AND estado!='DEVUELTO'`,
		now.Format(dateLayout), fee, id); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo procesar la devolución.")
		return
	}
	if _, err := tx.Exec(`UPDATE copias SET estado='DISPONIBLE' WHERE id=?`, copyID); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo liberar la copia.")
		return
	}
	result, err := tx.Exec(`INSERT INTO devoluciones(alquiler_id,fecha_devolucion,dias_retraso,recargo_calculado,observaciones,procesado_por_id) VALUES(?,?,?,?,?,?)`,
		id, now.Format(dateLayout), lateDays, fee, input.Observations, u.ID)
	if err != nil {
		log.Printf("create return record: %v", err)
		fail(w, http.StatusBadRequest, "Este alquiler ya fue devuelto previamente")
		return
	}
	returnID, _ := result.LastInsertId()
	if fee > 0 && input.ChargeNow {
		method := "EFECTIVO"
		if input.Method != nil {
			method = *input.Method
		}
		if !validPaymentMethod(method) {
			fail(w, http.StatusUnprocessableEntity, "Método de pago inválido.")
			return
		}
		if _, err := tx.Exec(`INSERT INTO pagos(alquiler_id,importe,metodo,fecha,estado,referencia) VALUES(?,?,?,?,?,?)`,
			id, fee, method, now.Format(dateLayout), "COMPLETADO", fmt.Sprintf("DEV-%05d-MORA", id)); err != nil {
			fail(w, http.StatusInternalServerError, "No se pudo registrar el pago del recargo.")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo confirmar la devolución.")
		return
	}
	var date string
	var notes sql.NullString
	_ = s.db.QueryRow(`SELECT fecha_devolucion,observaciones FROM devoluciones WHERE id=?`, returnID).Scan(&date, &notes)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": returnID, "alquiler_id": id, "fecha_devolucion": parseTime(date),
		"dias_retraso": lateDays, "recargo_calculado": fee, "observaciones": nullValue(notes),
	})
}

func (s *server) rentalPayments(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(w, r)
	if !ok {
		return
	}
	id, err := parseID(r)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "ID inválido.")
		return
	}
	var clientID int64
	if err := s.db.QueryRow(`SELECT cliente_id FROM alquileres WHERE id=?`, id).Scan(&clientID); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "Alquiler no encontrado")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los pagos.")
		return
	}
	if u.Role == "CLIENTE" && clientID != u.ID {
		fail(w, http.StatusForbidden, "No tiene permiso para ver los pagos de este alquiler.")
		return
	}
	s.paymentsForRental(w, id)
}

func (s *server) listPayments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	query := `SELECT id,alquiler_id,importe,metodo,fecha,estado,referencia FROM pagos WHERE 1=1`
	args := []any{}
	if raw := r.URL.Query().Get("alquiler_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "alquiler_id inválido.")
			return
		}
		query += ` AND alquiler_id=?`
		args = append(args, id)
	}
	query += ` ORDER BY fecha DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Printf("list payments: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los pagos.")
		return
	}
	defer rows.Close()
	payments := []map[string]any{}
	for rows.Next() {
		payment, err := scanPayment(rows)
		if err != nil {
			fail(w, http.StatusInternalServerError, "No se pudieron cargar los pagos.")
			return
		}
		payments = append(payments, payment)
	}
	writeJSON(w, http.StatusOK, payments)
}

func (s *server) paymentsForRental(w http.ResponseWriter, id int64) {
	rows, err := s.db.Query(`SELECT id,alquiler_id,importe,metodo,fecha,estado,referencia FROM pagos WHERE alquiler_id=?`, id)
	if err != nil {
		log.Printf("load rental payments: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudieron cargar los pagos.")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		payment, err := scanPayment(rows)
		if err != nil {
			fail(w, http.StatusInternalServerError, "No se pudieron cargar los pagos.")
			return
		}
		result = append(result, payment)
	}
	writeJSON(w, http.StatusOK, result)
}

func scanPayment(row copyRow) (map[string]any, error) {
	var id, rentalID int64
	var amount float64
	var method, date, state string
	var reference sql.NullString
	if err := row.Scan(&id, &rentalID, &amount, &method, &date, &state, &reference); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "alquiler_id": rentalID, "importe": amount,
		"metodo": method, "fecha": parseTime(date), "estado": state, "referencia": nullValue(reference)}, nil
}

func (s *server) createPayment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(w, r, "ADMIN", "EMPLEADO"); !ok {
		return
	}
	var input struct {
		RentalID  int64   `json:"alquiler_id"`
		Amount    float64 `json:"importe"`
		Method    string  `json:"metodo"`
		Reference *string `json:"referencia"`
		Type      string  `json:"tipo"`
	}
	if decodeBody(r, &input) != nil {
		fail(w, http.StatusUnprocessableEntity, "Datos de pago inválidos.")
		return
	}
	if input.Type == "" {
		input.Type = "ALQUILER"
	}
	if input.Amount <= 0 || !validPaymentMethod(input.Method) || (input.Type != "ALQUILER" && input.Type != "RECARGO") {
		fail(w, http.StatusUnprocessableEntity, "Datos de pago inválidos.")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM alquileres WHERE id=?`, input.RentalID).Scan(&exists); err != nil || exists == 0 {
		fail(w, http.StatusNotFound, "Alquiler no encontrado")
		return
	}
	now := time.Now().UTC()
	prefix := "ALQ-MAN"
	if input.Type == "RECARGO" {
		prefix = "DEV-MAN"
	}
	reference := fmt.Sprintf("%s-%05d-%d", prefix, input.RentalID, now.Unix())
	if input.Reference != nil && *input.Reference != "" {
		reference = fmt.Sprintf("%s-%05d-%s", prefix, input.RentalID, *input.Reference)
	}
	result, err := s.db.Exec(`INSERT INTO pagos(alquiler_id,importe,metodo,fecha,estado,referencia) VALUES(?,?,?,?,?,?)`,
		input.RentalID, input.Amount, input.Method, now.Format(dateLayout), "COMPLETADO", reference)
	if err != nil {
		log.Printf("create payment: %v", err)
		fail(w, http.StatusInternalServerError, "No se pudo registrar el pago.")
		return
	}
	id, _ := result.LastInsertId()
	var row map[string]any
	row, err = s.paymentByID(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "No se pudo cargar el pago registrado.")
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *server) paymentByID(id int64) (map[string]any, error) {
	row := s.db.QueryRow(`SELECT id,alquiler_id,importe,metodo,fecha,estado,referencia FROM pagos WHERE id=?`, id)
	return scanPayment(row)
}

func validPaymentMethod(method string) bool {
	return method == "EFECTIVO" || method == "TARJETA" || method == "TRANSFERENCIA"
}
