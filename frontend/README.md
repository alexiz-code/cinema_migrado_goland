# CINEMA

Sistema de alquiler de películas con frontend Next.js/React/TypeScript y backend Go/SQLite. El backend incluye la API de alquileres y administración, y mantiene los endpoints compatibles con CineMatch para que funcionen la búsqueda y las recomendaciones del frontend.

## Organización

- `src/peliculas/Peliculas.tsx`: pantalla de películas, búsqueda, carruseles, detalles y similares.
- `src/peliculas/peliculas.css`: estilos de la pantalla.
- `src/servicios/recomendador.ts`: cliente de la API CineMatch en el backend.
- `app/`: rutas y layout de Next.js.
- `backend/`: API Go, persistencia SQLite, autenticación, alquileres, pagos, inventario y endpoints compatibles con CineMatch.
- `docs/HISTORIAS_DE_USUARIO.md`: historias completas HU-01 a HU-11 y criterios de aceptación.

La interfaz incluye registro e inicio de sesión, catálogo filtrable, alquileres e historial, devoluciones y pagos para empleados, y administración de películas, inventario y reportes por rol. El alquiler no cobra en línea: los pagos permanecen pendientes hasta que el personal registre una recepción real.

## Requisitos

- Node.js 20.9 o posterior y npm
- Go 1.23 o posterior

## Ejecutar en Windows

Abre una terminal para el backend:

```powershell
cd "D:\6to semestre\cinema2\frontend\backend"
go run .
```

La primera ejecución crea y siembra `cinema.db` si todavía no existe. La API queda en `http://127.0.0.1:8001`; la documentación está disponible en `/docs`, `/redoc` y `/openapi.json`. Se pueden configurar `PORT`, `DATABASE_URL` y `SECRET_KEY` mediante variables de entorno.

Abre una segunda terminal para Next.js:

```powershell
cd "D:\6to semestre\cinema2\frontend"
npm install
npm run dev
```

Abre `http://localhost:3000/`. El backend debe permanecer ejecutándose para cargar películas y recomendaciones.

## Pruebas del backend

```powershell
cd "D:\6to semestre\cinema2\frontend\backend"
go test ./...
```

## Compilar el frontend

```bash
npm run build
```

Next.js genera los archivos de producción en `.next/`.
