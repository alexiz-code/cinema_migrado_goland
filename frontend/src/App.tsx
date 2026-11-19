'use client';

import { useEffect, useState } from 'react';
import AuthPage from './auth/AuthPage';
import Gestion from './gestion/Gestion';
import type { Section } from './gestion/Gestion';
import Peliculas from './peliculas/Peliculas';
import type { Session } from './servicios/api';

type Screen = Section | 'catalogo' | 'catalogo-admin' | 'acceso';

function readSession(): Session | null {
  const saved = sessionStorage.getItem('cinematch-session');
  if (!saved) return null;
  try {
    const session = JSON.parse(saved) as Session;
    return session.access_token && session.usuario ? session : null;
  } catch {
    sessionStorage.removeItem('cinematch-session');
    return null;
  }
}

export default function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [screen, setScreen] = useState<Screen>('catalogo');

  useEffect(() => {
    setSession(readSession());
  }, []);

  function authenticate(next: Session) {
    sessionStorage.setItem('cinematch-session', JSON.stringify(next));
    setSession(next);
    setScreen('catalogo');
  }

  function logout() {
    sessionStorage.removeItem('cinematch-session');
    setSession(null);
    setScreen('catalogo');
  }

  const links: { id: Screen; label: string }[] = [
    { id: 'catalogo', label: 'Catálogo' },
    ...(session?.usuario.rol === 'CLIENTE'
      ? [{ id: 'alquileres' as const, label: 'Mis alquileres' }]
      : []),
    ...(session?.usuario.rol === 'EMPLEADO'
      ? [
          { id: 'alquileres' as const, label: 'Devoluciones' },
          { id: 'pagos' as const, label: 'Pagos' },
          { id: 'inventario' as const, label: 'Inventario' },
        ]
      : []),
    ...(session?.usuario.rol === 'ADMIN'
      ? [
          { id: 'catalogo-admin' as const, label: 'Películas' },
          { id: 'inventario' as const, label: 'Inventario' },
          { id: 'alquileres' as const, label: 'Alquileres' },
          { id: 'pagos' as const, label: 'Pagos' },
          { id: 'reportes' as const, label: 'Reportes' },
        ]
      : []),
    ...(session ? [{ id: 'cuenta' as const, label: 'Mi cuenta' }] : []),
  ];

  return (
    <div className="app-shell">
      <header className="app-header">
        <button className="brand-button" type="button" onClick={() => setScreen('catalogo')}>
          CINEMATCH
        </button>
        <nav className="app-nav" aria-label="Secciones">
          {links.map((link, index) => (
            <button
              key={`${link.id}-${index}`}
              className={`nav-button ${screen === link.id ? 'active' : ''}`}
              type="button"
              onClick={() => setScreen(link.id)}
            >
              {link.label}
            </button>
          ))}
        </nav>
        <div className="account-area">
          {session ? (
            <>
              <span>
                {session.usuario.nombre} · {session.usuario.rol}
              </span>
              <button className="nav-button" type="button" onClick={logout}>
                Salir
              </button>
            </>
          ) : (
            <button className="nav-button" type="button" onClick={() => setScreen('acceso')}>
              Entrar / Registrarme
            </button>
          )}
        </div>
      </header>

      <main className="app-content">
        {screen === 'acceso' ? (
          <AuthPage onAuthenticated={authenticate} onCancel={() => setScreen('catalogo')} />
        ) : screen === 'catalogo' ? (
          <Peliculas token={session?.access_token} onLoginRequest={() => setScreen('acceso')} />
        ) : session ? (
          <Gestion
            token={session.access_token}
            role={session.usuario.rol}
            section={screen === 'catalogo-admin' ? 'catalogo' : screen}
            user={session.usuario}
            onUserUpdated={(usuario) => {
              const updated = { ...session, usuario };
              sessionStorage.setItem('cinematch-session', JSON.stringify(updated));
              setSession(updated);
            }}
          />
        ) : (
          <AuthPage onAuthenticated={authenticate} onCancel={() => setScreen('catalogo')} />
        )}
      </main>
    </div>
  );
}
