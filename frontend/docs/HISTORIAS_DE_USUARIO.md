# Historias de usuario — CINEMA

## Alcance

El sistema usa una interfaz React/TypeScript y una API FastAPI con SQLite. La autorización del backend es obligatoria: ocultar una opción en React no sustituye la validación del rol en cada endpoint. Los pagos que aparecen como `COMPLETADO` son pagos recibidos y registrados por un empleado; no representan un cargo automático ni una transacción de tarjeta.

## HU-01. Registro de clientes

**Prioridad:** Alta  
**Como** visitante, **quiero** crear una cuenta de cliente, **para** poder alquilar películas y consultar mis operaciones.

**Criterios de aceptación**

1. El formulario solicita nombre, correo y contraseña; el teléfono es opcional.
2. El navegador y la API validan los campos obligatorios, formato de correo, longitud del nombre y contraseña mínima de seis caracteres.
3. El correo se normaliza sin distinguir mayúsculas y no se permite registrar uno que ya exista.
4. El registro público asigna siempre el rol `CLIENTE`; nunca permite elegir `ADMIN` o `EMPLEADO`.
5. La contraseña se almacena únicamente como hash seguro, nunca en texto plano.
6. Si el registro funciona, la API devuelve una sesión autenticada y el usuario puede alquilar.
7. Si los datos son inválidos o el correo ya existe, se muestra el error y no se crea la cuenta.

**Pruebas de aceptación**

- Registrar una cuenta válida permite iniciar una sesión.
- Repetir el correo, incluso cambiando mayúsculas, produce un error de duplicado.
- Enviar una contraseña corta o correo inválido es rechazado.
- Un visitante no puede asignarse un rol privilegiado.

## HU-02. Inicio de sesión y autorización

**Prioridad:** Alta  
**Como** usuario registrado, **quiero** iniciar sesión, **para** usar las funciones autorizadas a mi rol.

**Criterios de aceptación**

1. El formulario solicita correo y contraseña.
2. La API verifica las credenciales y el estado activo de la cuenta.
3. Una autenticación correcta entrega un token de acceso y los datos básicos del usuario; la sesión de la interfaz se conserva en la pestaña.
4. El menú muestra funciones según el rol: cliente, empleado o administrador.
5. Las operaciones protegidas envían el token y la API rechaza credenciales inválidas, sesiones vencidas y roles no autorizados.
6. Cerrar sesión elimina la sesión local.
7. Los errores de acceso se comunican sin revelar si el correo existe.

**Pruebas de aceptación**

- Credenciales correctas permiten acceder a las funciones del rol.
- Contraseña incorrecta y cuenta inactiva no permiten entrar.
- Un cliente no puede usar endpoints administrativos aunque intente invocarlos directamente.
- Después de cerrar sesión, una operación protegida requiere volver a autenticarse.

## HU-03. Consultar y filtrar películas

**Prioridad:** Alta  
**Como** visitante o cliente, **quiero** explorar y filtrar el catálogo, **para** elegir una película que pueda alquilar.

**Criterios de aceptación**

1. El catálogo muestra título, sinopsis, año, géneros, calificación, tarifa diaria y copias disponibles.
2. Se puede filtrar por coincidencia de título, género y disponibilidad; los filtros se pueden combinar.
3. El detalle muestra la información de la película y el precio antes de confirmar un alquiler.
4. Las películas inactivas no aparecen en el catálogo público.
5. Si no hay resultados, se comunica claramente; los errores del servidor no se presentan como resultados vacíos.
6. Se puede consultar tendencias, mejor calificadas y películas similares desde la interfaz.

**Pruebas de aceptación**

- Buscar una parte del título devuelve las coincidencias.
- Aplicar género y disponibilidad a la vez devuelve únicamente películas que satisfacen ambos filtros.
- Una película sin copias disponibles no ofrece el flujo para alquilar.
- Una película desactivada no es visible para clientes.

## HU-04. Alquilar una película

**Prioridad:** Crítica  
**Como** cliente autenticado, **quiero** seleccionar una película disponible y los días de alquiler, **para** reservar una copia y conocer el importe.

**Criterios de aceptación**

1. Solo un cliente autenticado puede crear un alquiler para sí mismo desde el catálogo.
2. El periodo es obligatorio y está limitado de 1 a 30 días; se muestra el cálculo `tarifa diaria × días`.
3. El backend vuelve a comprobar la película, el periodo y la disponibilidad al confirmar.
4. La asignación de copia y la creación del alquiler son atómicas; una copia no puede asignarse a dos alquileres simultáneos.
5. El alquiler almacena fecha, fecha prevista de devolución, periodo, precio y estado.
6. Sin pasarela integrada, el sistema no intenta cobrar: registra el importe como pendiente y deja visible que el pago debe recibirse y registrarse por el personal.
7. Si no hay copias o la copia deja de estar disponible antes de confirmar, no se crea el alquiler y se muestra el motivo.

**Pruebas de aceptación**

- Un alquiler válido reserva una copia y calcula el importe correctamente.
- Intentar alquilar una copia ya ocupada no crea un segundo alquiler.
- Un visitante debe iniciar sesión antes de confirmar.
- El registro no aparece como dinero cobrado antes de que un empleado registre la recepción.

## HU-05. Consultar mis alquileres

**Prioridad:** Alta  
**Como** cliente, **quiero** ver mis alquileres actuales e históricos, **para** consultar fechas, estados, importes y posibles retrasos.

**Criterios de aceptación**

1. Solo se muestran alquileres asociados al usuario autenticado.
2. Cada registro presenta película, estado, días contratados, fecha prevista, importe y pagos registrados.
3. Los alquileres activos vencidos se identifican como `VENCIDO` y muestran días de retraso y recargo estimado conforme a la tarifa vigente.
4. El cliente no puede consultar registros de otro usuario modificando un identificador.

**Pruebas de aceptación**

- Un cliente ve su historial y no el de otro cliente.
- Un alquiler devuelto conserva sus fechas e importe en el historial.
- Al pasar la fecha prevista, el retraso y su estimación se reflejan al consultar el alquiler.

## HU-06. Gestionar devoluciones

**Prioridad:** Alta  
**Como** empleado, **quiero** registrar la devolución física de una copia, **para** cerrar el alquiler, actualizar inventario y calcular retrasos.

**Criterios de aceptación**

1. Solo `EMPLEADO` o `ADMIN` puede registrar devoluciones.
2. La operación registra fecha real, observaciones, días de retraso y recargo.
3. El recargo se calcula con la tarifa diaria de mora configurada en el sistema (o la tarifa predeterminada si no existe configuración).
4. La copia devuelta vuelve a estar disponible; el catálogo refleja el cambio.
5. Un alquiler ya devuelto no puede devolverse por segunda vez.
6. El recargo se registra como adeudado; no se afirma que fue cobrado automáticamente. Si se recibe el pago, el personal lo registra por separado.

**Pruebas de aceptación**

- Una devolución a tiempo registra cero días y cero recargo.
- Una devolución tardía registra el número de días y el cálculo de la tarifa.
- La copia queda disponible tras una devolución válida.
- Un cliente o visitante no puede procesar devoluciones.

## HU-07. Administrar películas

**Prioridad:** Alta  
**Como** administrador, **quiero** crear, editar, activar y desactivar películas, **para** mantener el catálogo sin perder su historial.

**Criterios de aceptación**

1. Solo `ADMIN` puede modificar el catálogo.
2. Los campos obligatorios se validan; año, duración, calificación y tarifa respetan sus límites.
3. Una película nueva puede inicializar copias físicas.
4. Editar una película conserva sus alquileres anteriores.
5. La baja es lógica (`INACTIVO`); no existe una acción de borrado que elimine registros relacionados.
6. Una película inactiva no se puede alquilar, aunque se intente llamar directamente a la API.

**Pruebas de aceptación**

- Datos válidos crean y actualizan películas.
- Tarifa cero o negativa, año fuera de rango o calificación inválida se rechazan.
- Desactivar una película con historial no elimina alquileres ni pagos.
- Un usuario que no sea administrador recibe una respuesta de acceso denegado.

## HU-08. Controlar inventario

**Prioridad:** Alta  
**Como** administrador, **quiero** registrar copias físicas y actualizar su estado, **para** conocer la disponibilidad real por película.

**Criterios de aceptación**

1. Cada copia tiene película asociada, código de inventario único y estado.
2. El administrador puede agregar copias y gestionar su ubicación y estado.
3. El empleado puede actualizar estados operativos permitidos; no puede crear copias.
4. Los estados incluyen `DISPONIBLE`, `ALQUILADA`, `MANTENIMIENTO` y `DADA_DE_BAJA`.
5. La disponibilidad del catálogo se calcula desde las copias disponibles, no de un contador manual.
6. El sistema impide registrar dos copias con el mismo código.

**Pruebas de aceptación**

- Registrar una copia aumenta la disponibilidad de la película.
- Marcar una copia como mantenimiento reduce la disponibilidad.
- Un código repetido se rechaza.
- Un empleado no puede agregar una copia ni modificar inventario si carece de permisos.

## HU-09. Registrar pagos

**Prioridad:** Media  
**Como** empleado, **quiero** registrar pagos recibidos y asociarlos a un alquiler, **para** controlar importes pendientes y operaciones confirmadas.

**Criterios de aceptación**

1. Solo empleado o administrador puede consultar y registrar pagos.
2. Cada pago tiene alquiler, importe positivo, método, fecha, estado y referencia opcional.
3. El pago inicial del alquiler queda `PENDIENTE`; no se simula una transacción de tarjeta.
4. El empleado registra un pago como recibido únicamente después de confirmar una recepción real fuera de línea (por ejemplo, efectivo o transferencia confirmada).
5. La interfaz aclara que no hay pasarela ni cobro electrónico integrado.
6. Los reportes de ingresos cuentan solo pagos registrados como `COMPLETADO`, no importes pendientes.
7. El personal clasifica los pagos recibidos como importe del alquiler o recargo para que los reportes separen correctamente ambos conceptos.

**Pruebas de aceptación**

- Un importe cero o negativo se rechaza.
- Un alquiler nuevo aparece con pago pendiente.
- Un pago pendiente no incrementa los ingresos cobrados.
- Un pago recibido queda asociado al alquiler, con método y referencia.

## HU-10. Consultar reportes

**Prioridad:** Media  
**Como** administrador, **quiero** consultar indicadores de actividad e ingresos filtrados por fechas, **para** tomar decisiones con base en datos registrados.

**Criterios de aceptación**

1. Solo `ADMIN` puede acceder a reportes.
2. El resumen presenta usuarios, películas, copias, alquileres activos/vencidos y pagos completados.
3. El desglose de ingresos se puede filtrar por fecha inicial y final.
4. El ranking de películas respeta el mismo periodo y presenta alquileres e ingresos realmente registrados.
5. Los pagos pendientes no se suman a los ingresos; los recargos adeudados no se cuentan como recaudados.
6. Un periodo sin operaciones devuelve resultados vacíos y totales en cero, no datos inventados.

**Pruebas de aceptación**

- Cambiar el periodo cambia el desglose y el ranking según las fechas de los registros.
- Un pago pendiente no aparece como ingreso.
- Un usuario sin rol `ADMIN` no puede consultar reportes.

## HU-11. Gestionar perfil y contraseña

**Prioridad:** Media  
**Como** usuario autenticado, **quiero** actualizar mis datos de contacto y cambiar mi contraseña, **para** mantener mi cuenta al día y protegerla.

**Criterios de aceptación**

1. El usuario puede editar su nombre y teléfono; su correo no se cambia desde este formulario.
2. Cambiar la contraseña requiere la contraseña actual y una nueva contraseña válida de al menos seis caracteres.
3. El backend verifica la contraseña actual y almacena la nueva como hash.
4. La operación no modifica rol, estado, alquileres ni pagos.
5. Los errores de validación se muestran y los cambios no autorizados se rechazan.

**Pruebas de aceptación**

- Guardar datos válidos actualiza el perfil.
- Una contraseña actual incorrecta no cambia la contraseña.
- Una contraseña nueva corta es rechazada.
- El token no permite modificar el rol mediante el endpoint de perfil.

## Reglas transversales

- Los datos y permisos se validan en el backend; la interfaz por rol es una ayuda de navegación.
- Los errores HTTP se muestran al usuario y no se convierten silenciosamente en listas vacías.
- Los importes se muestran con dos decimales y se calculan/validan también en la API.
- Ninguna función declara que se cobró dinero sin una recepción registrada; no hay integración de pasarela de pago.
- Los datos persistidos en SQLite sobreviven al reinicio del servidor.
