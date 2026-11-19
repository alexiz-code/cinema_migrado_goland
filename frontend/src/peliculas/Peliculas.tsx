import { useCallback, useEffect, useRef, useState } from 'react';
import type { MouseEvent, SubmitEvent } from 'react';
import { api } from '../servicios/api';
import type { PaymentMethod } from '../servicios/api';
import { recomendador } from '../servicios/recomendador';
import type { Pelicula } from '../servicios/recomendador';

type Seccion = {
  titulo: string;
  peliculas: Pelicula[];
};

type Props = {
  token?: string;
  onLoginRequest: () => void;
};

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Ocurrió un error inesperado.';
}

export default function Peliculas({ token, onLoginRequest }: Props) {
  const [cargando, setCargando] = useState(true);
  const [modalVisible, setModalVisible] = useState(false);
  const [peliculaSeleccionada, setPeliculaSeleccionada] = useState<Pelicula | null>(null);
  const [busqueda, setBusqueda] = useState('');
  const [genero, setGenero] = useState('');
  const [soloDisponibles, setSoloDisponibles] = useState(false);
  const [diasAlquiler, setDiasAlquiler] = useState(3);
  const [metodoPago, setMetodoPago] = useState<PaymentMethod>('EFECTIVO');
  const [alquilerVisible, setAlquilerVisible] = useState(false);
  const [mensajeAlquiler, setMensajeAlquiler] = useState('');
  const [secciones, setSecciones] = useState<Seccion[]>([]);
  const [errores, setErrores] = useState<string[]>([]);
  const busquedaRef = useRef<HTMLInputElement>(null);
  const solicitudActual = useRef(0);

  const cargarInicio = useCallback(async () => {
    const solicitud = ++solicitudActual.current;
    setCargando(true);
    setModalVisible(false);
    setBusqueda('');
    setSecciones([]);
    setErrores([]);
    setPeliculaSeleccionada(null);

    try {
      const peliculas = await recomendador.getCatalogo({}, token);
      if (solicitud !== solicitudActual.current) return;

      setSecciones([{ titulo: 'Todas las películas', peliculas }]);
      setPeliculaSeleccionada(peliculas[0] ?? null);
      setCargando(false);

      const cargasExtra: { titulo: string; cargar: () => Promise<Pelicula[]> }[] = [
        { titulo: 'Tendencias ahora', cargar: () => recomendador.getTendencias() },
        { titulo: 'Acción', cargar: () => recomendador.getPorGenero('Action') },
        { titulo: 'Aventura', cargar: () => recomendador.getPorGenero('Adventure') },
        { titulo: 'Ciencia ficción', cargar: () => recomendador.getPorGenero('Science Fiction') },
        { titulo: 'Terror', cargar: () => recomendador.getPorGenero('Horror') },
        { titulo: 'Comedia', cargar: () => recomendador.getPorGenero('Comedy') },
        { titulo: 'Mejor calificadas', cargar: () => recomendador.getMejorCalificadas() },
      ];

      for (const carga of cargasExtra) {
        void carga
          .cargar()
          .then((lista) => {
            if (solicitud !== solicitudActual.current || lista.length === 0) return;
            setSecciones((actuales) => [...actuales, { titulo: carga.titulo, peliculas: lista }]);
          })
          .catch((error: unknown) => {
            if (solicitud !== solicitudActual.current) return;
            console.error(`Error al cargar ${carga.titulo}:`, error);
            setErrores((actuales) => [...actuales, `${carga.titulo}: ${errorMessage(error)}`]);
          });
      }
    } catch (error) {
      if (solicitud !== solicitudActual.current) return;
      console.error('Error al cargar películas:', error);
      setErrores([`No se pudieron cargar las películas: ${errorMessage(error)}`]);
      setCargando(false);
    }
  }, [token]);

  useEffect(() => {
    void cargarInicio();
    return () => {
      solicitudActual.current += 1;
    };
  }, [cargarInicio]);

  function seleccionarPelicula(pelicula: Pelicula) {
    setPeliculaSeleccionada(pelicula);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  function abrirModal(pelicula: Pelicula) {
    setPeliculaSeleccionada(pelicula);
    setModalVisible(true);
  }

  async function mostrarSimilares(pelicula: Pelicula) {
    const solicitud = ++solicitudActual.current;
    setCargando(true);
    setModalVisible(false);
    setSecciones([]);
    setErrores([]);
    setPeliculaSeleccionada(pelicula);

    try {
      const peliculas = await recomendador.getSimilares(pelicula.titulo);
      if (solicitud !== solicitudActual.current) return;

      setSecciones([{ titulo: `Películas similares a ${pelicula.titulo}`, peliculas }]);
      setPeliculaSeleccionada(peliculas[0] ?? pelicula);
    } catch (error) {
      if (solicitud !== solicitudActual.current) return;
      console.error('Error al cargar películas similares:', error);
      setErrores([`No se pudieron cargar películas similares: ${errorMessage(error)}`]);
    } finally {
      if (solicitud === solicitudActual.current) setCargando(false);
    }
  }

  async function buscarPeliculas(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const texto = busqueda.trim();

    if (texto === '' && !genero && !soloDisponibles) {
      await cargarInicio();
      return;
    }

    const solicitud = ++solicitudActual.current;
    setCargando(true);
    setSecciones([]);
    setErrores([]);
    setPeliculaSeleccionada(null);

    try {
      const peliculas = await recomendador.getCatalogo(
        { titulo: texto || undefined, genero: genero || undefined, disponible: soloDisponibles },
        token,
      );
      if (solicitud !== solicitudActual.current) return;

      setSecciones([
        {
          titulo: texto ? `Resultados para: ${texto}` : 'Películas filtradas',
          peliculas,
        },
      ]);
      setPeliculaSeleccionada(peliculas[0] ?? null);
    } catch (error) {
      if (solicitud !== solicitudActual.current) return;
      console.error('Error al buscar películas:', error);
      setErrores([`No se pudo completar la búsqueda: ${errorMessage(error)}`]);
    } finally {
      if (solicitud === solicitudActual.current) setCargando(false);
    }
  }

  async function alquilarPelicula(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!peliculaSeleccionada) return;
    if (!token) {
      onLoginRequest();
      return;
    }

    try {
      const alquiler = await api.createRental(token, {
        pelicula_id: peliculaSeleccionada.id,
        dias_alquiler: diasAlquiler,
        metodo_pago: metodoPago,
      });
      setMensajeAlquiler(
        `Alquiler #${alquiler.id} registrado. Total: $${alquiler.precio_alquiler.toFixed(2)}. ` +
          'El pago queda pendiente; no se procesó ningún cobro en línea.',
      );
      setAlquilerVisible(false);
      await cargarInicio();
    } catch (error) {
      console.error('No se pudo registrar el alquiler:', error);
      setMensajeAlquiler(errorMessage(error));
    }
  }

  function reproducir() {
    if (!peliculaSeleccionada) return;

    window.open(
      `https://www.youtube.com/results?search_query=${encodeURIComponent(
        `${peliculaSeleccionada.titulo} trailer español`,
      )}`,
      '_blank',
      'noopener,noreferrer',
    );
  }

  function moverCarrusel(event: MouseEvent<HTMLButtonElement>, direccion: number) {
    const carrusel = event.currentTarget.closest('.fila')?.querySelector('.carrusel');
    carrusel?.scrollBy({ left: direccion * 700, behavior: 'smooth' });
  }

  return (
    <div className="netflix">
      {cargando ? (
        <div className="loading">Cargando películas...</div>
      ) : (
        <main className="contenido">
          <header className="topbar">
            <h2>CineMatch</h2>
            <form className="buscador" onSubmit={buscarPeliculas}>
              <input
                ref={busquedaRef}
                type="text"
                autoComplete="off"
                spellCheck={false}
                placeholder="Buscar película..."
                value={busqueda}
                onChange={(event) => setBusqueda(event.target.value)}
              />
              <select
                aria-label="Filtrar por género"
                value={genero}
                onChange={(event) => setGenero(event.target.value)}
              >
                <option value="">Todos los géneros</option>
                <option value="Action">Acción</option>
                <option value="Adventure">Aventura</option>
                <option value="Comedy">Comedia</option>
                <option value="Horror">Terror</option>
                <option value="Science Fiction">Ciencia ficción</option>
                <option value="Drama">Drama</option>
              </select>
              <label className="availability-filter">
                <input
                  type="checkbox"
                  checked={soloDisponibles}
                  onChange={(event) => setSoloDisponibles(event.target.checked)}
                />
                Disponibles
              </label>
              <button type="submit">Buscar</button>
            </form>
          </header>

          {mensajeAlquiler && (
            <div className="notice" role="status">
              {mensajeAlquiler}
            </div>
          )}

          {peliculaSeleccionada && (
            <section
              className="hero"
              style={{
                backgroundImage: peliculaSeleccionada.poster
                  ? `url("${peliculaSeleccionada.poster}")`
                  : undefined,
              }}
            >
              <div className="hero-info">
                <div className="marca">N PELÍCULA</div>
                <h1>{peliculaSeleccionada.titulo}</h1>
                <div className="meta">
                  <span>{peliculaSeleccionada.anio}</span>
                  <span>⭐ {peliculaSeleccionada.rating}</span>
                  <span>{peliculaSeleccionada.generos}</span>
                </div>
                <p>{peliculaSeleccionada.descripcion}</p>
                <div className="botones">
                  <button className="btn-play" type="button" onClick={reproducir}>
                    ▶ Reproducir
                  </button>
                  <button
                    className="btn-info"
                    type="button"
                    onClick={() => abrirModal(peliculaSeleccionada)}
                  >
                    ⓘ Más información
                  </button>
                  <button
                    className="btn-similar"
                    type="button"
                    onClick={() => void mostrarSimilares(peliculaSeleccionada)}
                  >
                    ★ Ver similares
                  </button>
                  <button
                    className="btn-play"
                    type="button"
                    onClick={() => {
                      if (!token) onLoginRequest();
                      else setAlquilerVisible(true);
                    }}
                    disabled={peliculaSeleccionada.disponibles === 0}
                  >
                    {peliculaSeleccionada.disponibles === 0 ? 'Sin copias disponibles' : 'Alquilar'}
                  </button>
                </div>
              </div>
            </section>
          )}

          {(!peliculaSeleccionada || secciones.length === 0) && (
            <section className="sin-resultados">No se encontraron películas.</section>
          )}

          {errores.length > 0 && (
            <div className="errores" role="alert">
              {errores.map((error, index) => (
                <p key={`${error}-${index}`}>{error}</p>
              ))}
            </div>
          )}

          {secciones.map((seccion) => (
            <section className="fila" key={seccion.titulo}>
              <div className="fila-cabecera">
                <h2>{seccion.titulo}</h2>
                <div className="flechas">
                  <button
                    type="button"
                    aria-label={`Desplazar ${seccion.titulo} a la izquierda`}
                    onClick={(event) => moverCarrusel(event, -1)}
                  >
                    ‹
                  </button>
                  <button
                    type="button"
                    aria-label={`Desplazar ${seccion.titulo} a la derecha`}
                    onClick={(event) => moverCarrusel(event, 1)}
                  >
                    ›
                  </button>
                </div>
              </div>

              <div className="carrusel">
                {seccion.peliculas.map((pelicula, index) => (
                  <div
                    className="poster"
                    key={`${pelicula.titulo}-${index}`}
                    onClick={() => seleccionarPelicula(pelicula)}
                  >
                    <img src={pelicula.poster} alt={pelicula.titulo} />
                    <div className="poster-info">
                      <h3>{pelicula.titulo}</h3>
                      <div className="poster-meta">
                        <span>{pelicula.anio}</span>
                        <span>⭐ {pelicula.rating}</span>
                      </div>
                      <p className="catalog-price">
                        ${pelicula.precio_dia.toFixed(2)} / día · {pelicula.disponibles} disponibles
                      </p>
                      <div className="acciones-card" onClick={(event) => event.stopPropagation()}>
                        <button type="button" onClick={() => abrirModal(pelicula)}>
                          Info
                        </button>
                        <button type="button" onClick={() => void mostrarSimilares(pelicula)}>
                          Similares
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </main>
      )}

      {modalVisible && peliculaSeleccionada && (
        <div className="modal-fondo" onClick={() => setModalVisible(false)}>
          <div className="modal" onClick={(event) => event.stopPropagation()}>
            <button
              type="button"
              className="cerrar"
              aria-label="Cerrar"
              onClick={() => setModalVisible(false)}
            >
              ×
            </button>
            <img src={peliculaSeleccionada.poster} alt={peliculaSeleccionada.titulo} />
            <div className="modal-info">
              <h2>{peliculaSeleccionada.titulo}</h2>
              <p className="modal-meta">
                {peliculaSeleccionada.anio} | ⭐ {peliculaSeleccionada.rating} | Popularidad:{' '}
                {peliculaSeleccionada.popularidad}
              </p>
              <p>{peliculaSeleccionada.descripcion}</p>
              <p>
                <strong>Géneros:</strong> {peliculaSeleccionada.generos}
              </p>
              <div className="modal-botones">
                <button className="btn-play" type="button" onClick={reproducir}>
                  ▶ Ver tráiler
                </button>
                <button
                  className="btn-similar"
                  type="button"
                  onClick={() => void mostrarSimilares(peliculaSeleccionada)}
                >
                  ★ Similares
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {alquilerVisible && peliculaSeleccionada && (
        <div className="modal-fondo" onClick={() => setAlquilerVisible(false)}>
          <section className="action-modal" onClick={(event) => event.stopPropagation()}>
            <button
              type="button"
              className="cerrar"
              aria-label="Cerrar"
              onClick={() => setAlquilerVisible(false)}
            >
              ×
            </button>
            <h2>Alquilar {peliculaSeleccionada.titulo}</h2>
            <p>
              Tarifa: ${peliculaSeleccionada.precio_dia.toFixed(2)} por día ·{' '}
              {peliculaSeleccionada.disponibles} copias disponibles
            </p>
            <form className="form-grid" onSubmit={alquilarPelicula}>
              <label>
                Días de alquiler
                <input
                  type="number"
                  min={1}
                  max={30}
                  required
                  value={diasAlquiler}
                  onChange={(event) => setDiasAlquiler(Number(event.target.value))}
                />
              </label>
              <label>
                Método previsto
                <select
                  value={metodoPago}
                  onChange={(event) => setMetodoPago(event.target.value as PaymentMethod)}
                >
                  <option value="EFECTIVO">Efectivo (pagar en local)</option>
                  <option value="TRANSFERENCIA">Transferencia (registrar al recibir)</option>
                  <option value="TARJETA">Tarjeta (sin cobro en línea)</option>
                </select>
              </label>
              <p className="total-preview">
                Total a registrar: ${(peliculaSeleccionada.precio_dia * diasAlquiler).toFixed(2)}
              </p>
              <p className="panel-description">
                Este registro no cobra dinero. El pago queda pendiente hasta que el personal
                confirme su recepción.
              </p>
              <button className="btn-play" type="submit">
                Confirmar alquiler
              </button>
            </form>
          </section>
        </div>
      )}
    </div>
  );
}
