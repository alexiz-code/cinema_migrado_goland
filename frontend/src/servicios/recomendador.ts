import { api } from './api';
import type { Movie } from './api';

export type Pelicula = {
  id: number;
  titulo: string;
  anio?: string;
  rating?: string;
  generos?: string;
  descripcion?: string;
  poster?: string;
  popularidad?: string;
  precio_dia: number;
  disponibles: number;
  copias_totales: number;
  estado?: 'ACTIVO' | 'INACTIVO';
};

const API_URL = 'http://127.0.0.1:8001';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function optionalText(value: unknown): string | undefined {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }

  if (Array.isArray(value) && value.every((item) => typeof item === 'string')) {
    return value.join(', ');
  }

  return undefined;
}

function optionalNumber(value: unknown): number | undefined {
  if (typeof value === 'number') return value;
  if (typeof value === 'string' && value.trim() !== '' && Number.isFinite(Number(value))) {
    return Number(value);
  }
  return undefined;
}

function parsePeliculas(value: unknown): Pelicula[] {
  if (!Array.isArray(value)) {
    throw new Error('El servidor devolvió una respuesta de películas inválida.');
  }

  return value.map((item, index) => {
    if (!isRecord(item) || typeof item.titulo !== 'string') {
      throw new Error(`La película en la posición ${index + 1} no tiene un título válido.`);
    }

    return {
      id: optionalNumber(item.id) ?? index + 1,
      titulo: item.titulo,
      anio: optionalText(item.anio),
      rating: optionalText(item.rating),
      generos: optionalText(item.generos),
      descripcion: optionalText(item.descripcion),
      poster: optionalText(item.poster),
      popularidad: optionalText(item.popularidad),
      precio_dia: optionalNumber(item.precio_dia) ?? 0,
      disponibles: optionalNumber(item.disponibles ?? item.copias_disponibles) ?? 0,
      copias_totales: optionalNumber(item.copias_totales) ?? 0,
      estado: item.estado === 'INACTIVO' ? 'INACTIVO' : 'ACTIVO',
    };
  });
}

async function solicitarPeliculas(path: string, body?: object): Promise<Pelicula[]> {
  const response = await fetch(`${API_URL}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(8000),
  });

  if (!response.ok) {
    throw new Error(`El servidor respondió con el estado ${response.status}.`);
  }

  return parsePeliculas(await response.json());
}

export const recomendador = {
  getCatalogo(
    filtros: { titulo?: string; genero?: string; disponible?: boolean; incluirInactivas?: boolean },
    token?: string,
  ): Promise<Pelicula[]> {
    return api.listMovies(filtros, token).then((movies: Movie[]) =>
      movies.map((movie) => ({
        id: movie.id,
        titulo: movie.titulo,
        anio: String(movie.anio),
        rating: String(movie.rating),
        generos: movie.generos,
        descripcion: movie.descripcion,
        poster: movie.poster,
        popularidad: String(movie.popularidad),
        precio_dia: movie.precio_dia,
        disponibles: movie.copias_disponibles ?? movie.disponibles ?? 0,
        copias_totales: movie.copias_totales ?? 0,
        estado: movie.estado,
      })),
    );
  },

  getRecomendaciones(gustos: string[]): Promise<Pelicula[]> {
    return solicitarPeliculas('/recomendar', { gustos });
  },

  getSimilares(titulo: string): Promise<Pelicula[]> {
    return solicitarPeliculas('/similar', { titulo });
  },

  getPeliculas(): Promise<Pelicula[]> {
    return solicitarPeliculas('/peliculas');
  },

  getTendencias(): Promise<Pelicula[]> {
    return solicitarPeliculas('/tendencias');
  },

  getMejorCalificadas(): Promise<Pelicula[]> {
    return solicitarPeliculas('/mejor-calificadas');
  },

  getPorGenero(genero: string): Promise<Pelicula[]> {
    return solicitarPeliculas(`/genero/${encodeURIComponent(genero)}`);
  },

  buscar(consulta: string): Promise<Pelicula[]> {
    return solicitarPeliculas(`/buscar?q=${encodeURIComponent(consulta)}`);
  },
};
