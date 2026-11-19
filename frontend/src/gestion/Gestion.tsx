import { useCallback, useEffect, useState } from 'react';
import type { SubmitEvent } from 'react';
import { api } from '../servicios/api';
import type {
  Copy,
  CopyStatus,
  Movie,
  MovieInput,
  Payment,
  PaymentMethod,
  Rental,
  ReportSummary,
  RevenueReport,
  Role,
  User,
} from '../servicios/api';

export type Section = 'alquileres' | 'inventario' | 'catalogo' | 'pagos' | 'reportes' | 'cuenta';

type Props = {
  token: string;
  role: Role;
  section: Section;
  user: User;
  onUserUpdated: (user: User) => void;
};

const rentalDate = (date: string) => new Date(date).toLocaleDateString('es');
const money = (value: number) => `$${value.toFixed(2)}`;

export default function Gestion({ token, role, section, user, onUserUpdated }: Props) {
  const [rentals, setRentals] = useState<Rental[]>([]);
  const [copies, setCopies] = useState<Copy[]>([]);
  const [movies, setMovies] = useState<Movie[]>([]);
  const [payments, setPayments] = useState<Payment[]>([]);
  const [summary, setSummary] = useState<ReportSummary | null>(null);
  const [revenue, setRevenue] = useState<RevenueReport | null>(null);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [editingMovieId, setEditingMovieId] = useState<number | null>(null);

  const report = useCallback(async () => {
    const [summaryData, revenueData] = await Promise.all([
      api.reportSummary(token),
      api.revenueReport(token),
    ]);
    setSummary(summaryData);
    setRevenue(revenueData);
  }, [token]);

  const load = useCallback(async () => {
    setError('');
    try {
      if (section === 'alquileres') {
        setRentals(role === 'CLIENTE' ? await api.myRentals(token) : await api.allRentals(token));
      } else if (section === 'inventario') {
        const [copyData, movieData] = await Promise.all([
          api.listCopies(token),
          api.listMovies({ incluirInactivas: true }, token),
        ]);
        setCopies(copyData);
        setMovies(movieData);
      } else if (section === 'catalogo') {
        setMovies(await api.listMovies({ incluirInactivas: true }, token));
      } else if (section === 'pagos') {
        setPayments(await api.listPayments(token));
        if (role !== 'CLIENTE') setRentals(await api.allRentals(token));
      } else if (section === 'reportes') {
        await report();
      }
    } catch (caught) {
      console.error('No se pudo cargar la sección:', caught);
      setError(caught instanceof Error ? caught.message : 'No se pudieron cargar los datos.');
    }
  }, [report, role, section, token]);

  useEffect(() => {
    void load();
  }, [load]);

  async function runAction(action: () => Promise<unknown>, success: string): Promise<boolean> {
    setError('');
    setNotice('');
    try {
      await action();
      setNotice(success);
      await load();
      return true;
    } catch (caught) {
      console.error('La operación no pudo completarse:', caught);
      setError(caught instanceof Error ? caught.message : 'La operación no pudo completarse.');
      return false;
    }
  }

  async function handleReturn(event: SubmitEvent<HTMLFormElement>, rentalId: number) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    await runAction(
      () => api.returnRental(token, rentalId, String(data.get('observaciones') || '')),
      `Devolución del alquiler #${rentalId} registrada. El recargo, si aplica, queda pendiente.`,
    );
  }

  async function handlePayment(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const saved = await runAction(
      () =>
        api.recordPayment(token, {
          alquiler_id: Number(data.get('alquiler_id')),
          importe: Number(data.get('importe')),
          metodo: String(data.get('metodo')) as PaymentMethod,
          referencia: String(data.get('referencia') || ''),
          tipo: String(data.get('tipo')) as 'ALQUILER' | 'RECARGO',
        }),
      'Pago recibido registrado. No se procesó un cobro electrónico.',
    );
    if (saved) form.reset();
  }

  async function handleMovie(event: SubmitEvent<HTMLFormElement>, movie?: Movie) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const input: MovieInput = {
      titulo: String(data.get('titulo')),
      sinopsis: String(data.get('sinopsis')),
      anio: Number(data.get('anio')),
      duracion: Number(data.get('duracion')),
      clasificacion: String(data.get('clasificacion')),
      generos: String(data.get('generos')),
      poster_url: String(data.get('poster_url')),
      precio_dia: Number(data.get('precio_dia')),
      rating: Number(data.get('rating')),
      popularidad: Number(data.get('popularidad')),
      copias_iniciales: Number(data.get('copias_iniciales') || 0),
    };
    const saved = await runAction(
      () => (movie ? api.updateMovie(token, movie.id, input) : api.createMovie(token, input)),
      movie ? 'Película actualizada.' : 'Película creada.',
    );
    if (saved) setEditingMovieId(null);
  }

  const showNotice = notice && (
    <p className="notice" role="status">
      {notice}
    </p>
  );
  const showError = error && (
    <p className="form-error" role="alert">
      {error}
    </p>
  );

  if (section === 'cuenta') {
    async function saveProfile(event: SubmitEvent<HTMLFormElement>) {
      event.preventDefault();
      const data = new FormData(event.currentTarget);
      await runAction(async () => {
        const updated = await api.updateProfile(token, {
          nombre: String(data.get('nombre')),
          telefono: String(data.get('telefono') || ''),
        });
        onUserUpdated(updated);
      }, 'Perfil actualizado.');
    }

    async function changePassword(event: SubmitEvent<HTMLFormElement>) {
      event.preventDefault();
      const form = event.currentTarget;
      const data = new FormData(form);
      const saved = await runAction(
        () =>
          api.changePassword(
            token,
            String(data.get('contrasena_actual')),
            String(data.get('nueva_contrasena')),
          ),
        'Contraseña actualizada.',
      );
      if (saved) form.reset();
    }

    return (
      <section className="management-panel">
        <h1>Mi cuenta</h1>
        {showNotice}
        {showError}
        <form className="form-grid compact-form" onSubmit={saveProfile}>
          <label>
            Nombre
            <input
              name="nombre"
              required
              minLength={2}
              maxLength={120}
              defaultValue={user.nombre}
            />
          </label>
          <label>
            Correo
            <input type="email" value={user.correo} disabled />
          </label>
          <label>
            Teléfono
            <input name="telefono" maxLength={30} defaultValue={user.telefono ?? ''} />
          </label>
          <button className="btn-play" type="submit">
            Guardar perfil
          </button>
        </form>
        <form className="form-grid compact-form" onSubmit={changePassword}>
          <h2>Cambiar contraseña</h2>
          <label>
            Contraseña actual
            <input
              name="contrasena_actual"
              type="password"
              required
              autoComplete="current-password"
            />
          </label>
          <label>
            Nueva contraseña
            <input
              name="nueva_contrasena"
              type="password"
              required
              minLength={6}
              autoComplete="new-password"
            />
          </label>
          <button className="btn-play" type="submit">
            Actualizar contraseña
          </button>
        </form>
      </section>
    );
  }

  if (section === 'alquileres') {
    return (
      <section className="management-panel">
        <h1>{role === 'CLIENTE' ? 'Mis alquileres' : 'Alquileres y devoluciones'}</h1>
        <p className="panel-description">
          Consulta las fechas, el estado, el importe registrado y los recargos por retraso.
        </p>
        {showNotice}
        {showError}
        {rentals.length === 0 ? (
          <p>No hay alquileres registrados.</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Película</th>
                  {role !== 'CLIENTE' && <th>Cliente</th>}
                  <th>Estado</th>
                  <th>Entrega prevista</th>
                  <th>Días</th>
                  <th>Importe</th>
                  <th>Recargo</th>
                  {role !== 'CLIENTE' && <th>Acción</th>}
                </tr>
              </thead>
              <tbody>
                {rentals.map((rental) => (
                  <tr key={rental.id}>
                    <td>
                      {rental.pelicula_titulo} <small>#{rental.id}</small>
                    </td>
                    {role !== 'CLIENTE' && (
                      <td>
                        {rental.cliente_nombre}
                        <small>{rental.cliente_correo}</small>
                      </td>
                    )}
                    <td>{rental.estado}</td>
                    <td>{rentalDate(rental.fecha_prevista_devolucion)}</td>
                    <td>{rental.dias_alquiler}</td>
                    <td>
                      {money(rental.precio_alquiler)}{' '}
                      <small>pagado: {money(rental.total_pagado)}</small>
                    </td>
                    <td>
                      {money(rental.recargo || rental.recargo_estimado)}
                      {rental.dias_retraso_actual > 0 && (
                        <small>{rental.dias_retraso_actual} día(s) de retraso</small>
                      )}
                    </td>
                    {role !== 'CLIENTE' && (
                      <td>
                        {rental.estado !== 'DEVUELTO' && rental.estado !== 'CANCELADO' ? (
                          <form
                            className="inline-form"
                            onSubmit={(event) => void handleReturn(event, rental.id)}
                          >
                            <input
                              name="observaciones"
                              placeholder="Observaciones"
                              aria-label="Observaciones"
                            />
                            <button className="btn-similar" type="submit">
                              Registrar devolución
                            </button>
                          </form>
                        ) : (
                          'Devuelto'
                        )}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    );
  }

  if (section === 'pagos') {
    return (
      <section className="management-panel">
        <h1>Registro de pagos</h1>
        <p className="panel-description">
          Registra un pago recibido en efectivo o fuera de línea. Esta pantalla no procesa tarjetas
          ni realiza cargos.
        </p>
        {role !== 'CLIENTE' && (
          <form className="form-grid compact-form" onSubmit={handlePayment}>
            <label>
              Alquiler
              <select name="alquiler_id" required defaultValue="">
                <option value="" disabled>
                  Selecciona alquiler
                </option>
                {rentals.map((rental) => (
                  <option key={rental.id} value={rental.id}>
                    #{rental.id} · {rental.pelicula_titulo} · {rental.cliente_nombre}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Importe
              <input name="importe" type="number" min="0.01" step="0.01" required />
            </label>
            <label>
              Método
              <select name="metodo" defaultValue="EFECTIVO">
                <option value="EFECTIVO">Efectivo</option>
                <option value="TRANSFERENCIA">Transferencia recibida</option>
                <option value="TARJETA">Tarjeta recibida fuera del sistema</option>
              </select>
            </label>
            <label>
              Tipo de pago
              <select name="tipo" defaultValue="ALQUILER">
                <option value="ALQUILER">Importe de alquiler</option>
                <option value="RECARGO">Recargo por retraso</option>
              </select>
            </label>
            <label>
              Referencia/recibo
              <input name="referencia" maxLength={100} />
            </label>
            <button className="btn-play" type="submit">
              Registrar pago recibido
            </button>
          </form>
        )}
        {showNotice}
        {showError}
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Alquiler</th>
                <th>Fecha</th>
                <th>Importe</th>
                <th>Método</th>
                <th>Estado</th>
                <th>Referencia</th>
              </tr>
            </thead>
            <tbody>
              {payments.map((payment) => (
                <tr key={payment.id}>
                  <td>#{payment.alquiler_id}</td>
                  <td>{rentalDate(payment.fecha)}</td>
                  <td>{money(payment.importe)}</td>
                  <td>{payment.metodo}</td>
                  <td>{payment.estado}</td>
                  <td>{payment.referencia}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    );
  }

  if (section === 'catalogo') {
    const editMovie = movies.find((movie) => movie.id === editingMovieId);
    return (
      <section className="management-panel">
        <h1>Administrar películas</h1>
        <p className="panel-description">
          Las películas se desactivan en lugar de eliminarse, para conservar su historial.
        </p>
        {showNotice}
        {showError}
        <form
          key={editMovie?.id ?? 'new-movie'}
          className="form-grid movie-form"
          onSubmit={(event) => void handleMovie(event, editMovie)}
        >
          <h2>{editMovie ? `Editar ${editMovie.titulo}` : 'Agregar película'}</h2>
          <label>
            Título
            <input name="titulo" required maxLength={180} defaultValue={editMovie?.titulo} />
          </label>
          <label>
            Sinopsis
            <textarea
              name="sinopsis"
              required
              defaultValue={editMovie?.descripcion ?? editMovie?.sinopsis}
            />
          </label>
          <label>
            Año
            <input
              name="anio"
              type="number"
              min="1890"
              max="2050"
              required
              defaultValue={editMovie?.anio ?? new Date().getFullYear()}
            />
          </label>
          <label>
            Duración (minutos)
            <input
              name="duracion"
              type="number"
              min="1"
              required
              defaultValue={editMovie?.duracion ?? 90}
            />
          </label>
          <label>
            Clasificación
            <input
              name="clasificacion"
              required
              defaultValue={editMovie?.clasificacion ?? 'PG-13'}
            />
          </label>
          <label>
            Géneros (separados por comas)
            <input name="generos" required defaultValue={editMovie?.generos} />
          </label>
          <label>
            URL del póster
            <input
              name="poster_url"
              type="url"
              required
              defaultValue={editMovie?.poster_url ?? editMovie?.poster}
            />
          </label>
          <label>
            Precio por día
            <input
              name="precio_dia"
              type="number"
              min="0.01"
              step="0.01"
              required
              defaultValue={editMovie?.precio_dia ?? 2.5}
            />
          </label>
          <label>
            Calificación (0-10)
            <input
              name="rating"
              type="number"
              min="0"
              max="10"
              step="0.1"
              required
              defaultValue={editMovie?.rating ?? 7}
            />
          </label>
          <label>
            Popularidad
            <input
              name="popularidad"
              type="number"
              min="0"
              step="0.1"
              required
              defaultValue={editMovie?.popularidad ?? 80}
            />
          </label>
          {!editMovie && (
            <label>
              Copias iniciales
              <input name="copias_iniciales" type="number" min="0" defaultValue={0} />
            </label>
          )}
          <button className="btn-play" type="submit">
            {editMovie ? 'Guardar cambios' : 'Crear película'}
          </button>
          {editMovie && (
            <button className="btn-info" type="button" onClick={() => setEditingMovieId(null)}>
              Cancelar edición
            </button>
          )}
        </form>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Título</th>
                <th>Estado</th>
                <th>Tarifa/día</th>
                <th>Disponibles</th>
                <th>Acciones</th>
              </tr>
            </thead>
            <tbody>
              {movies.map((movie) => (
                <tr key={movie.id}>
                  <td>{movie.titulo}</td>
                  <td>{movie.estado}</td>
                  <td>{money(movie.precio_dia)}</td>
                  <td>{movie.copias_disponibles ?? movie.disponibles}</td>
                  <td className="table-actions">
                    <button
                      type="button"
                      className="btn-info"
                      onClick={() => setEditingMovieId(movie.id)}
                    >
                      Editar
                    </button>
                    <button
                      type="button"
                      className="btn-similar"
                      onClick={() =>
                        void runAction(
                          () =>
                            api.setMovieStatus(
                              token,
                              movie.id,
                              movie.estado === 'INACTIVO' ? 'ACTIVO' : 'INACTIVO',
                            ),
                          movie.estado === 'INACTIVO'
                            ? 'Película reactivada.'
                            : 'Película desactivada sin borrar su historial.',
                        )
                      }
                    >
                      {movie.estado === 'INACTIVO' ? 'Reactivar' : 'Desactivar'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    );
  }

  if (section === 'inventario') {
    async function addCopy(event: SubmitEvent<HTMLFormElement>) {
      event.preventDefault();
      const form = event.currentTarget;
      const data = new FormData(form);
      const saved = await runAction(
        () =>
          api.createCopy(token, {
            pelicula_id: Number(data.get('pelicula_id')),
            codigo_inventario: String(data.get('codigo_inventario')),
            ubicacion: String(data.get('ubicacion')),
          }),
        'Copia agregada al inventario.',
      );
      if (saved) form.reset();
    }

    async function changeCopyStatus(copy: Copy, estado: CopyStatus) {
      await runAction(
        () => api.updateCopy(token, copy.id, { estado }),
        'Estado de copia actualizado.',
      );
    }

    return (
      <section className="management-panel">
        <h1>Control de inventario</h1>
        <p className="panel-description">
          Administra identificadores y estados de las copias físicas.
        </p>
        {showNotice}
        {showError}
        {role === 'ADMIN' && (
          <form className="form-grid compact-form" onSubmit={addCopy}>
            <label>
              Película
              <select name="pelicula_id" required defaultValue="">
                <option value="" disabled>
                  Selecciona película
                </option>
                {movies
                  .filter((movie) => movie.estado !== 'INACTIVO')
                  .map((movie) => (
                    <option key={movie.id} value={movie.id}>
                      {movie.titulo}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              Código de inventario
              <input name="codigo_inventario" required maxLength={50} />
            </label>
            <label>
              Ubicación
              <input name="ubicacion" required maxLength={80} defaultValue="Estante General" />
            </label>
            <button className="btn-play" type="submit">
              Agregar copia
            </button>
          </form>
        )}
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Código</th>
                <th>Película</th>
                <th>Ubicación</th>
                <th>Estado</th>
                <th>Cambiar estado</th>
              </tr>
            </thead>
            <tbody>
              {copies.map((copy) => (
                <tr key={copy.id}>
                  <td>{copy.codigo_inventario}</td>
                  <td>{copy.titulo_pelicula}</td>
                  <td>{copy.ubicacion}</td>
                  <td>{copy.estado}</td>
                  <td>
                    <select
                      value={copy.estado}
                      aria-label={`Estado de ${copy.codigo_inventario}`}
                      disabled={copy.estado === 'ALQUILADA'}
                      onChange={(event) =>
                        void changeCopyStatus(copy, event.target.value as CopyStatus)
                      }
                    >
                      <option value="DISPONIBLE">Disponible</option>
                      <option value="MANTENIMIENTO">Mantenimiento</option>
                      <option value="DADA_DE_BAJA">Dada de baja</option>
                      <option value="ALQUILADA" disabled>
                        Alquilada
                      </option>
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    );
  }

  async function filterRevenue(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    try {
      const result = await api.revenueReport(
        token,
        String(data.get('fecha_inicio') || ''),
        String(data.get('fecha_fin') || ''),
      );
      setRevenue(result);
      setError('');
    } catch (caught) {
      console.error('No se pudo filtrar el reporte:', caught);
      setError(caught instanceof Error ? caught.message : 'No se pudo filtrar el reporte.');
    }
  }

  return (
    <section className="management-panel">
      <h1>Reportes administrativos</h1>
      <p className="panel-description">
        Indicadores calculados desde los alquileres y pagos almacenados.
      </p>
      {showError}
      {summary && (
        <div className="kpi-grid">
          {[
            ['Clientes', summary.total_clientes],
            ['Películas', summary.total_peliculas],
            ['Copias disponibles', summary.copias_disponibles],
            ['Copias alquiladas', summary.copias_alquiladas],
            ['Alquileres activos', summary.alquileres_activos],
            ['Alquileres vencidos', summary.alquileres_vencidos],
            ['Ingresos registrados', money(summary.ingresos_totales)],
            ['Recargos registrados', money(summary.ingresos_recargos)],
          ].map(([label, value]) => (
            <article className="kpi-card" key={label}>
              <span>{label}</span>
              <strong>{value}</strong>
            </article>
          ))}
        </div>
      )}
      <form className="form-grid compact-form" onSubmit={filterRevenue}>
        <label>
          Desde
          <input name="fecha_inicio" type="date" />
        </label>
        <label>
          Hasta
          <input name="fecha_fin" type="date" />
        </label>
        <button className="btn-play" type="submit">
          Filtrar ingresos
        </button>
      </form>
      {revenue && (
        <>
          <h2>Ingresos en periodo: {money(revenue.ingresos_totales)}</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Fecha</th>
                  <th>Operaciones</th>
                  <th>Alquileres</th>
                  <th>Recargos</th>
                  <th>Total</th>
                </tr>
              </thead>
              <tbody>
                {revenue.desglose_por_fecha.map((item) => (
                  <tr key={item.fecha}>
                    <td>{item.fecha}</td>
                    <td>{item.alquileres_count}</td>
                    <td>{money(item.ingresos_alquiler)}</td>
                    <td>{money(item.ingresos_recargo)}</td>
                    <td>{money(item.total)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <h2>Películas más alquiladas</h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Película</th>
                  <th>Alquileres</th>
                  <th>Ingresos registrados</th>
                </tr>
              </thead>
              <tbody>
                {revenue.top_peliculas.map((movie) => (
                  <tr key={movie.pelicula_id}>
                    <td>{movie.titulo}</td>
                    <td>{movie.total_alquileres}</td>
                    <td>{money(movie.ingresos_generados)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}
