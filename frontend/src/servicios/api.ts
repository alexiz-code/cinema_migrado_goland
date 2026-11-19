const API_URL = 'http://127.0.0.1:8001';

export type Role = 'ADMIN' | 'EMPLEADO' | 'CLIENTE';
export type RentalStatus = 'ACTIVO' | 'DEVUELTO' | 'VENCIDO' | 'CANCELADO';
export type CopyStatus = 'DISPONIBLE' | 'ALQUILADA' | 'MANTENIMIENTO' | 'DADA_DE_BAJA';
export type PaymentMethod = 'EFECTIVO' | 'TARJETA' | 'TRANSFERENCIA';

export type User = {
  id: number;
  nombre: string;
  correo: string;
  telefono?: string | null;
  rol: Role;
  estado: 'ACTIVO' | 'INACTIVO';
};

export type Session = {
  access_token: string;
  token_type: string;
  usuario: User;
};

export type Movie = {
  id: number;
  titulo: string;
  descripcion?: string;
  sinopsis?: string;
  anio: number;
  duracion: number;
  clasificacion: string;
  generos: string;
  poster?: string;
  poster_url?: string;
  rating: number;
  popularidad: number;
  precio_dia: number;
  disponibles?: number;
  copias_totales?: number;
  copias_disponibles?: number;
  estado?: 'ACTIVO' | 'INACTIVO';
};

export type Rental = {
  id: number;
  cliente_id: number;
  cliente_nombre?: string;
  cliente_correo?: string;
  copia_id: number;
  codigo_copia?: string;
  pelicula_id: number;
  pelicula_titulo?: string;
  pelicula_poster?: string;
  fecha_alquiler: string;
  fecha_prevista_devolucion: string;
  fecha_real_devolucion?: string | null;
  dias_alquiler: number;
  precio_alquiler: number;
  recargo: number;
  total_pagado: number;
  estado: RentalStatus;
  dias_retraso_actual: number;
  recargo_estimado: number;
};

export type Copy = {
  id: number;
  pelicula_id: number;
  codigo_inventario: string;
  estado: CopyStatus;
  ubicacion?: string;
  observaciones?: string;
  titulo_pelicula?: string;
};

export type Payment = {
  id: number;
  alquiler_id: number;
  importe: number;
  metodo: PaymentMethod;
  fecha: string;
  estado: 'COMPLETADO' | 'PENDIENTE' | 'REEMBOLSADO';
  referencia?: string;
};

export type ReportSummary = {
  total_usuarios: number;
  total_clientes: number;
  total_peliculas: number;
  total_copias: number;
  copias_disponibles: number;
  copias_alquiladas: number;
  alquileres_activos: number;
  alquileres_vencidos: number;
  alquileres_totales: number;
  ingresos_totales: number;
  ingresos_recargos: number;
};

export type RevenueReport = {
  ingresos_totales: number;
  desglose_por_fecha: {
    fecha: string;
    alquileres_count: number;
    ingresos_alquiler: number;
    ingresos_recargo: number;
    total: number;
  }[];
  top_peliculas: {
    pelicula_id: number;
    titulo: string;
    poster_url: string;
    total_alquileres: number;
    ingresos_generados: number;
  }[];
};

export type MovieInput = {
  titulo: string;
  sinopsis: string;
  anio: number;
  duracion: number;
  clasificacion: string;
  generos: string;
  poster_url: string;
  precio_dia: number;
  rating: number;
  popularidad: number;
  copias_iniciales?: number;
};

export type ErrorResponse = {
  detail?: string | { msg?: string }[];
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, token?: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    headers,
    signal: init.signal ?? AbortSignal.timeout(10000),
  });

  if (!response.ok) {
    let message = `El servidor respondió con el estado ${response.status}.`;
    try {
      const payload = (await response.json()) as ErrorResponse;
      if (typeof payload.detail === 'string') {
        message = payload.detail;
      } else if (Array.isArray(payload.detail)) {
        message = payload.detail
          .map((issue) => issue.msg)
          .filter(Boolean)
          .join(' ');
      }
    } catch {
      // Keep the HTTP status message when the server response is not JSON.
    }
    throw new ApiError(message, response.status);
  }

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

function json(body: unknown): RequestInit {
  return { method: 'POST', body: JSON.stringify(body) };
}

export const api = {
  request,
  register: (data: { nombre: string; correo: string; contrasena: string; telefono?: string }) =>
    request<Session>('/api/auth/registro', undefined, json(data)),
  login: (correo: string, contrasena: string) =>
    request<Session>('/api/auth/login', undefined, json({ correo, contrasena })),
  profile: (token: string) => request<User>('/api/auth/me', token),
  updateProfile: (token: string, data: { nombre?: string; telefono?: string }) =>
    request<User>('/api/auth/perfil', token, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  changePassword: (token: string, contrasena_actual: string, nueva_contrasena: string) =>
    request<{ mensaje: string }>('/api/auth/cambiar-contrasena', token, {
      method: 'PUT',
      body: JSON.stringify({ contrasena_actual, nueva_contrasena }),
    }),
  listMovies: (
    params: {
      titulo?: string;
      genero?: string;
      disponible?: boolean;
      incluirInactivas?: boolean;
    } = {},
    token?: string,
  ): Promise<Movie[]> => {
    const query = new URLSearchParams();
    if (params.titulo) query.set('titulo', params.titulo);
    if (params.genero) query.set('genero', params.genero);
    if (params.disponible) query.set('disponible', 'true');
    if (params.incluirInactivas) query.set('incluir_inactivas', 'true');
    return request<
      Array<
        Omit<Movie, 'descripcion' | 'poster' | 'disponibles'> & {
          sinopsis: string;
          poster_url: string;
          copias_disponibles: number;
        }
      >
    >(`/api/peliculas${query.size ? `?${query.toString()}` : ''}`, token).then((movies) =>
      movies.map((movie) => ({
        ...movie,
        descripcion: movie.sinopsis,
        poster: movie.poster_url,
        disponibles: movie.copias_disponibles,
      })),
    );
  },
  movieAvailability: (id: number) =>
    request<{ disponibles: number; total_copias: number }>(`/api/peliculas/${id}/disponibilidad`),
  createRental: (
    token: string,
    data: { pelicula_id: number; dias_alquiler: number; metodo_pago: PaymentMethod },
  ) => request<Rental>('/api/alquileres', token, json(data)),
  myRentals: (token: string) => request<Rental[]>('/api/alquileres/mis-alquileres', token),
  allRentals: (token: string) => request<Rental[]>('/api/alquileres', token),
  returnRental: (token: string, id: number, observaciones: string) =>
    request('/api/alquileres/' + id + '/devolucion', token, {
      method: 'POST',
      body: JSON.stringify({ observaciones, cobrar_recargo_ahora: false }),
    }),
  listCopies: (token: string) => request<Copy[]>('/api/copias', token),
  createCopy: (
    token: string,
    data: { pelicula_id: number; codigo_inventario: string; ubicacion: string },
  ) => request<Copy>('/api/copias', token, json(data)),
  updateCopy: (
    token: string,
    id: number,
    data: { estado?: CopyStatus; ubicacion?: string; observaciones?: string },
  ) =>
    request<Copy>(`/api/copias/${id}`, token, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  createMovie: (token: string, data: MovieInput) =>
    request<Movie>('/api/peliculas', token, json(data)),
  updateMovie: (token: string, id: number, data: MovieInput) =>
    request<Movie>(`/api/peliculas/${id}`, token, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  setMovieStatus: (token: string, id: number, estado: 'ACTIVO' | 'INACTIVO') =>
    request<Movie>(`/api/peliculas/${id}/estado`, token, {
      method: 'PATCH',
      body: JSON.stringify({ estado }),
    }),
  listPayments: (token: string) => request<Payment[]>('/api/pagos', token),
  recordPayment: (
    token: string,
    data: {
      alquiler_id: number;
      importe: number;
      metodo: PaymentMethod;
      referencia?: string;
      tipo: 'ALQUILER' | 'RECARGO';
    },
  ) => request<Payment>('/api/pagos', token, json(data)),
  reportSummary: (token: string) => request<ReportSummary>('/api/admin/reportes/resumen', token),
  revenueReport: (token: string, from?: string, to?: string) => {
    const query = new URLSearchParams();
    if (from) query.set('fecha_inicio', from);
    if (to) query.set('fecha_fin', to);
    return request<RevenueReport>(
      `/api/admin/reportes/ingresos${query.size ? `?${query.toString()}` : ''}`,
      token,
    );
  },
};
