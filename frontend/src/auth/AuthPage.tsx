import { useState } from 'react';
import type { SubmitEvent } from 'react';
import { api } from '../servicios/api';
import type { Session } from '../servicios/api';

type Props = {
  onAuthenticated: (session: Session) => void;
  onCancel: () => void;
};

export default function AuthPage({ onAuthenticated, onCancel }: Props) {
  const [mode, setMode] = useState<'login' | 'registro'>('login');
  const [error, setError] = useState('');
  const [working, setWorking] = useState(false);

  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setError('');
    setWorking(true);
    const data = new FormData(event.currentTarget);

    try {
      const session =
        mode === 'login'
          ? await api.login(String(data.get('correo')), String(data.get('contrasena')))
          : await api.register({
              nombre: String(data.get('nombre')),
              correo: String(data.get('correo')),
              contrasena: String(data.get('contrasena')),
              telefono: String(data.get('telefono') || ''),
            });
      onAuthenticated(session);
    } catch (caught) {
      console.error('No se pudo autenticar al usuario:', caught);
      setError(caught instanceof Error ? caught.message : 'No se pudo completar la solicitud.');
    } finally {
      setWorking(false);
    }
  }

  return (
    <section className="auth-card">
      <p className="marca">CINEMATCH</p>
      <h1>{mode === 'login' ? 'Iniciar sesión' : 'Crear cuenta de cliente'}</h1>
      <p className="panel-description">
        {mode === 'login'
          ? 'Accede a las funciones disponibles para tu cuenta.'
          : 'Tus datos se validan y la contraseña se almacena cifrada.'}
      </p>
      <form className="form-grid" onSubmit={submit}>
        {mode === 'registro' && (
          <>
            <label>
              Nombre
              <input name="nombre" required minLength={2} maxLength={120} autoComplete="name" />
            </label>
            <label>
              Teléfono (opcional)
              <input name="telefono" maxLength={30} autoComplete="tel" />
            </label>
          </>
        )}
        <label>
          Correo electrónico
          <input name="correo" type="email" required maxLength={120} autoComplete="email" />
        </label>
        <label>
          Contraseña
          <input
            name="contrasena"
            type="password"
            required
            minLength={mode === 'registro' ? 6 : 4}
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
          />
        </label>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <div className="form-actions">
          <button className="btn-play" type="submit" disabled={working}>
            {working ? 'Procesando...' : mode === 'login' ? 'Entrar' : 'Registrarme'}
          </button>
          <button className="btn-info" type="button" onClick={onCancel}>
            Volver al catálogo
          </button>
        </div>
      </form>
      <button
        className="link-button"
        type="button"
        onClick={() => {
          setError('');
          setMode(mode === 'login' ? 'registro' : 'login');
        }}
      >
        {mode === 'login' ? '¿No tienes cuenta? Regístrate' : 'Ya tengo cuenta'}
      </button>
    </section>
  );
}
