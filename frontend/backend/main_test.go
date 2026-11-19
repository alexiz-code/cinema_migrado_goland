package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"
)

func testServer(t *testing.T) *server {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	app := &server{db: db, secret: []byte(defaultSecret), port: "8001"}
	if err := app.initialize(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return app
}

func requestJSON(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var content bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&content).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &content)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestHealthAndRentalLifecycle(t *testing.T) {
	app := testServer(t)
	handler := app.routes()

	health := requestJSON(t, handler, http.MethodGet, "/api/health", "", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", health.Code)
	}
	docs := requestJSON(t, handler, http.MethodGet, "/docs", "", nil)
	if docs.Code != http.StatusOK {
		t.Fatalf("docs status = %d, want 200", docs.Code)
	}
	openAPI := requestJSON(t, handler, http.MethodGet, "/openapi.json", "", nil)
	if openAPI.Code != http.StatusOK || !json.Valid(openAPI.Body.Bytes()) {
		t.Fatalf("OpenAPI status/body invalid: %d", openAPI.Code)
	}

	login := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"correo": "cliente@cinema.com", "contrasena": "cliente123",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body)
	}
	var session struct {
		Token string `json:"access_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("login did not return a token: %v", err)
	}

	rental := requestJSON(t, handler, http.MethodPost, "/api/alquileres", session.Token, map[string]any{
		"pelicula_id": 1, "dias_alquiler": 3,
	})
	if rental.Code != http.StatusCreated {
		t.Fatalf("rental status = %d, body = %s", rental.Code, rental.Body)
	}
	var created struct {
		ID       int64   `json:"id"`
		Price    float64 `json:"precio_alquiler"`
		CopyCode string  `json:"codigo_copia"`
	}
	if err := json.Unmarshal(rental.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID < 1 || created.Price <= 0 || created.CopyCode == "" {
		t.Fatalf("incomplete rental response: %+v", created)
	}
	var copyState string
	if err := app.db.QueryRow(`SELECT estado FROM copias WHERE codigo_inventario=?`, created.CopyCode).Scan(&copyState); err != nil {
		t.Fatal(err)
	}
	if copyState != "ALQUILADA" {
		t.Fatalf("copy state = %s, want ALQUILADA", copyState)
	}
}

func TestPublicRegistrationAlwaysCreatesClientRole(t *testing.T) {
	app := testServer(t)
	response := requestJSON(t, app.routes(), http.MethodPost, "/api/auth/registro", "", map[string]string{
		"nombre": "Nuevo Cliente", "correo": "new@example.com",
		"contrasena": "secreto123", "rol": "ADMIN",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("registration status = %d, body = %s", response.Code, response.Body)
	}
	var payload struct {
		User struct {
			Role string `json:"rol"`
		} `json:"usuario"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.User.Role != "CLIENTE" {
		t.Fatalf("registered role = %q, want CLIENTE", payload.User.Role)
	}
}

func loginToken(t *testing.T, handler http.Handler, email, password string) string {
	t.Helper()
	response := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"correo": email, "contrasena": password,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("login for %s returned %d: %s", email, response.Code, response.Body)
	}
	var result struct {
		Token string `json:"access_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Token
}

func TestInventoryReturnAndAdministrativeReports(t *testing.T) {
	app := testServer(t)
	handler := app.routes()
	adminToken := loginToken(t, handler, "admin@cinema.com", "admin123")
	clientToken := loginToken(t, handler, "cliente@cinema.com", "cliente123")
	employeeToken := loginToken(t, handler, "empleado@cinema.com", "empleado123")

	catalog := requestJSON(t, handler, http.MethodGet, "/api/peliculas?disponible=true", "", nil)
	if catalog.Code != http.StatusOK {
		t.Fatalf("list movies returned %d: %s", catalog.Code, catalog.Body)
	}
	compatCatalog := requestJSON(t, handler, http.MethodGet, "/peliculas", "", nil)
	if compatCatalog.Code != http.StatusOK {
		t.Fatalf("CineMatch compatibility catalog returned %d: %s", compatCatalog.Code, compatCatalog.Body)
	}
	rentals := requestJSON(t, handler, http.MethodGet, "/api/alquileres", clientToken, nil)
	if rentals.Code != http.StatusOK {
		t.Fatalf("list rentals returned %d: %s", rentals.Code, rentals.Body)
	}

	movie := requestJSON(t, handler, http.MethodPost, "/api/peliculas", adminToken, map[string]any{
		"titulo": "Go Test Movie", "sinopsis": "Película de prueba", "anio": 2024,
		"duracion": 100, "clasificacion": "PG", "generos": "Drama",
		"poster_url": "https://example.com/poster.jpg", "precio_dia": 2.5,
		"copias_iniciales": 2,
	})
	if movie.Code != http.StatusCreated {
		t.Fatalf("create movie returned %d: %s", movie.Code, movie.Body)
	}
	var created struct {
		ID        int64 `json:"id"`
		Total     int   `json:"copias_totales"`
		Available int   `json:"copias_disponibles"`
	}
	if err := json.Unmarshal(movie.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID < 1 || created.Total != 2 || created.Available != 2 {
		t.Fatalf("created movie inventory = %+v", created)
	}
	invalidUpdate := requestJSON(t, handler, http.MethodPut,
		"/api/peliculas/"+strconv.FormatInt(created.ID, 10), adminToken, map[string]any{"precio_dia": 0})
	if invalidUpdate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid movie update returned %d, want 422", invalidUpdate.Code)
	}

	rental := requestJSON(t, handler, http.MethodPost, "/api/alquileres", clientToken,
		map[string]any{"pelicula_id": created.ID, "dias_alquiler": 2})
	if rental.Code != http.StatusCreated {
		t.Fatalf("create rental returned %d: %s", rental.Code, rental.Body)
	}
	var createdRental struct {
		ID     int64 `json:"id"`
		CopyID int64 `json:"copia_id"`
	}
	if err := json.Unmarshal(rental.Body.Bytes(), &createdRental); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`UPDATE alquileres SET fecha_prevista_devolucion='2000-01-01 00:00:00' WHERE id=?`, createdRental.ID); err != nil {
		t.Fatal(err)
	}
	returned := requestJSON(t, handler, http.MethodPost,
		"/api/alquileres/"+strconv.FormatInt(createdRental.ID, 10)+"/devolucion",
		employeeToken, map[string]any{"cobrar_recargo_ahora": true})
	if returned.Code != http.StatusOK {
		t.Fatalf("return rental returned %d: %s", returned.Code, returned.Body)
	}
	var returnedData struct {
		LateDays int     `json:"dias_retraso"`
		Fee      float64 `json:"recargo_calculado"`
	}
	if err := json.Unmarshal(returned.Body.Bytes(), &returnedData); err != nil {
		t.Fatal(err)
	}
	if returnedData.LateDays < 1 || returnedData.Fee <= 0 {
		t.Fatalf("return did not calculate late fee: %+v", returnedData)
	}

	copyState := requestJSON(t, handler, http.MethodGet, "/api/copias", employeeToken, nil)
	if copyState.Code != http.StatusOK {
		t.Fatalf("list copies returned %d: %s", copyState.Code, copyState.Body)
	}
	var summary struct {
		TotalMovies int64   `json:"total_peliculas"`
		Revenue     float64 `json:"ingresos_totales"`
	}
	report := requestJSON(t, handler, http.MethodGet, "/api/admin/reportes/resumen", adminToken, nil)
	if report.Code != http.StatusOK {
		t.Fatalf("summary report returned %d: %s", report.Code, report.Body)
	}
	if err := json.Unmarshal(report.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.TotalMovies < 1 || summary.Revenue <= 0 {
		t.Fatalf("unexpected report summary: %+v", summary)
	}
	rentalsReport := requestJSON(t, handler, http.MethodGet, "/api/admin/reportes/alquileres", adminToken, nil)
	if rentalsReport.Code != http.StatusOK {
		t.Fatalf("rentals report returned %d: %s", rentalsReport.Code, rentalsReport.Body)
	}
}
