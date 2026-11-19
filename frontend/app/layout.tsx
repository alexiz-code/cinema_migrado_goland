import type { Metadata } from 'next';
import type { ReactNode } from 'react';
import '../src/shell.css';
import '../src/peliculas/peliculas.css';

export const metadata: Metadata = {
  title: 'CineMatch',
  description: 'Catálogo y alquiler de películas',
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="es">
      <body>{children}</body>
    </html>
  );
}
