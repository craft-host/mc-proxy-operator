# Minecraft Proxy Operator para Kubernetes — Multi-Edition

## Descripción General

Este proyecto implementa un **Kubernetes Operator** escrito en **Go** que gestiona un **reverse proxy multi-protocolo para servidores de Minecraft**. Soporta múltiples ediciones de Minecraft (Java Edition, Bedrock Edition, y extensible a futuras ediciones) mediante el **Strategy Pattern**, donde cada edición tiene su propia implementación de protocolo.

El operator permite que múltiples usuarios tengan sus propios servidores Minecraft dentro de un clúster de Kubernetes, accesibles a través de subdominios personalizados, independientemente de la edición de Minecraft que usen.

### Caso de uso

Este proxy es el componente central de un **servicio de venta/hosting de servidores de Minecraft**. Cada cliente compra un servidor, y el sistema automáticamente:
1. Despliega el servidor MC del cliente en Kubernetes
2. Crea un `MinecraftProxy` CR que configura la ruta
3. El proxy empieza a enrutar tráfico al servidor del cliente

### Flujo de tráfico

```
┌─────────────────────────────────────────────────────────────────┐
│                     Jugadores externos                          │
│                                                                 │
│  Java Player ──TCP──►  jugador1.miserver.com:25565              │
│  Bedrock Player ─UDP─► jugador2.miserver.com:19132              │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
              ┌──────────────────────────┐
              │   MC Proxy Operator Pod   │
              │                          │
              │  ┌────────────────────┐  │
              │  │  Protocol Router   │  │
              │  └────────┬───────────┘  │
              │           │              │
              │  ┌────────┴───────────┐  │
              │  │                    │  │
              │  ▼                    ▼  │
              │ JavaHandler     BedrockHandler
              │ (TCP :25565)    (UDP :19132)  │
              │  │                    │  │
              └──┼────────────────────┼──┘
                 │                    │
        ┌────────┴──┐          ┌─────┴──────┐
        ▼           ▼          ▼            ▼
  svc/jugador1  svc/jugador3  svc/jugador2  svc/jugador4
    :25565        :25565        :19132        :19132
   (Java)        (Java)       (Bedrock)     (Bedrock)
```

---

## Ediciones de Minecraft Soportadas

| Edición | Protocolo | Puerto Default | Método de extracción del hostname |
|---|---|---|---|
| **Java Edition** | TCP | 25565 | Handshake Packet (ID 0x00) — el hostname está en el campo `Server Address` del primer paquete |
| **Bedrock Edition** | UDP (RakNet) | 19132 | Unconnected Ping — el hostname se envía en la respuesta del Unconnected Pong; el routing se basa en la dirección destino del paquete UDP |

### Notas sobre Bedrock Edition y UDP Routing

A diferencia de Java Edition donde el cliente envía el hostname en el handshake, **Bedrock Edition usa UDP con RakNet** y el protocolo no incluye un campo de hostname en el paquete inicial del cliente. Esto significa que el routing en Bedrock se implementa de forma diferente:

**Estrategia de routing para Bedrock:**
1. **Opción A — Puerto único por usuario:** Cada servidor Bedrock se asigna un puerto UDP diferente (ej: 19132, 19133, 19134...). El proxy escucha en un rango de puertos y rutea por puerto. Esto es simple pero limita la escalabilidad y requiere que los jugadores sepan su puerto.
2. **Opción B — IP por usuario con múltiples IPs en el LoadBalancer:** Se asigna una IP diferente por servidor. El proxy rutea por IP destino.
3. **Opción C (recomendada) — Subdomain → puerto dinámico con DNS:** El jugador se conecta a `jugador2.miserver.com:19132`. El DNS resuelve a la IP del proxy. El proxy mantiene un mapeo de IP origen → backend basado en una fase de "registro" donde el cliente Bedrock primero llama a un API HTTP para registrar su IP, y luego se conecta al proxy que ya sabe a dónde rutear.
4. **Opción D (más práctica para producción) — Puerto por usuario:** Cada `MinecraftProxy` de tipo Bedrock obtiene un puerto UDP dedicado en el rango 19132-29132. El proxy abre un listener UDP por cada servidor Bedrock. El CRD incluye el puerto asignado, y el DNS SRV o un portal web le indica al jugador su puerto.

**Para esta implementación, se usa la Opción D (puerto por usuario para Bedrock)** por ser la más práctica y la que usan servicios reales de hosting MC. El campo `spec.assignedPort` en el CRD almacena el puerto asignado para servidores Bedrock.

---

## Arquitectura: Strategy Pattern

### Diagrama de la interfaz

```
                    ┌──────────────────────────┐
                    │   ProtocolHandler         │  ← Interface
                    │                          │
                    │  + Name() string         │
                    │  + DefaultPort() int32   │
                    │  + Start(ctx, rt) error  │
                    │  + Stop() error          │
                    └────────────┬─────────────┘
                                 │
                    ┌────────────┼────────────────┐
                    │            │                 │
                    ▼            ▼                 ▼
            ┌──────────┐  ┌───────────┐   ┌──────────────┐
            │  Java     │  │  Bedrock  │   │  Future      │
            │  Handler  │  │  Handler  │   │  Handler     │
            │           │  │           │   │  (ej: Geyser)│
            │  TCP      │  │  UDP      │   │              │
            │  :25565   │  │  :19132+  │   │              │
            └──────────┘  └───────────┘   └──────────────┘
```

### Registro de handlers

```
┌─────────────────────────────────────┐
│         HandlerRegistry             │
│                                     │
│  "java"    → JavaProtocolHandler    │
│  "bedrock" → BedrockProtocolHandler │
│  "geyser"  → (futuro)              │
│                                     │
│  + Register(edition, handler)       │
│  + Get(edition) → handler           │
│  + ListEditions() → []string        │
└─────────────────────────────────────┘
```

---

## Tecnologías y Dependencias

| Tecnología | Versión | Propósito |
|---|---|---|
| Go | >= 1.22 | Lenguaje principal |
| Kubebuilder | v4 | Scaffolding del operator |
| controller-runtime | v0.19+ | Framework del controller/reconciler |
| Kubernetes | >= 1.28 | Plataforma de despliegue |
| Docker | latest | Construcción de imagen |

### Dependencias Go (go.mod)

```
sigs.k8s.io/controller-runtime  → Framework del operator
k8s.io/apimachinery              → Tipos de Kubernetes
k8s.io/client-go                 → Cliente de Kubernetes
```

**NO se requieren dependencias externas** para los proxies TCP/UDP ni para los parsers de protocolos. Todo se implementa con la biblioteca estándar de Go (`net`, `io`, `encoding/binary`, `sync`).

---

## Estructura del Proyecto

```
mc-proxy-operator/
├── api/
│   └── v1alpha1/
│       ├── minecraftproxy_types.go       # Definición del CRD (Spec, Status)
│       ├── groupversion_info.go          # Registro del API group
│       └── zz_generated.deepcopy.go      # Generado automáticamente
│
├── cmd/
│   └── main.go                           # Entrypoint: arranca manager + handlers
│
├── internal/
│   ├── controller/
│   │   ├── minecraftproxy_controller.go  # Reconciler del CRD
│   │   └── minecraftproxy_controller_test.go
│   │
│   └── proxy/
│       ├── handler.go                    # Interface ProtocolHandler + Registry
│       ├── handler_test.go               # Tests del registry
│       ├── routetable.go                 # Tabla de rutas thread-safe
│       ├── routetable_test.go            # Tests de la route table
│       │
│       ├── java/
│       │   ├── handler.go               # JavaProtocolHandler (TCP proxy)
│       │   ├── minecraft.go             # Parser del Handshake packet MC Java
│       │   ├── minecraft_test.go        # Tests del parser Java
│       │   └── handler_test.go          # Tests de integración Java
│       │
│       ├── bedrock/
│       │   ├── handler.go               # BedrockProtocolHandler (UDP proxy)
│       │   ├── raknet.go                # Parser de paquetes RakNet básicos
│       │   ├── raknet_test.go           # Tests del parser RakNet
│       │   └── handler_test.go          # Tests de integración Bedrock
│       │
│       └── portmanager/
│           ├── manager.go               # Gestión de puertos dinámicos para Bedrock
│           └── manager_test.go
│
├── config/
│   ├── crd/
│   │   └── bases/                        # CRD YAML generado
│   ├── rbac/
│   │   ├── role.yaml
│   │   └── role_binding.yaml
│   ├── manager/
│   │   └── manager.yaml                  # Deployment del operator
│   └── samples/
│       ├── minecraft_v1alpha1_java.yaml       # Ejemplo Java
│       └── minecraft_v1alpha1_bedrock.yaml    # Ejemplo Bedrock
│
├── deploy/
│   ├── namespace.yaml
│   ├── service-java.yaml                 # Service LoadBalancer TCP :25565
│   ├── service-bedrock.yaml              # Service LoadBalancer UDP :19132-19232
│   └── examples/
│       ├── java-server-jugador1.yaml
│       └── bedrock-server-jugador2.yaml
│
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## Paso 1: Scaffolding con Kubebuilder

### 1.1 Inicializar el proyecto

```bash
mkdir mc-proxy-operator && cd mc-proxy-operator
kubebuilder init --domain miminecraftserver.com --repo github.com/tuuser/mc-proxy-operator
```

### 1.2 Crear el API y Controller

```bash
kubebuilder create api \
  --group minecraft \
  --version v1alpha1 \
  --kind MinecraftProxy \
  --resource \
  --controller
```

### 1.3 Verificar compilación

```bash
make generate
make manifests
go build ./...
```

---

## Paso 2: Definir el CRD con soporte Multi-Edition

### 2.1 Archivo: `api/v1alpha1/minecraftproxy_types.go`

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Edition representa la edición de Minecraft soportada.
// +kubebuilder:validation:Enum=java;bedrock
type Edition string

const (
	// EditionJava es Minecraft Java Edition (TCP, puerto default 25565)
	EditionJava Edition = "java"

	// EditionBedrock es Minecraft Bedrock Edition (UDP/RakNet, puerto default 19132)
	EditionBedrock Edition = "bedrock"
)

// MinecraftProxySpec define la configuración deseada para una ruta de proxy.
type MinecraftProxySpec struct {
	// Edition especifica la edición de Minecraft que usa este servidor.
	// Determina qué protocolo y handler se usa para el proxy.
	// Valores válidos: "java", "bedrock"
	//
	// - "java": Usa TCP en puerto 25565. El hostname se extrae del Handshake Packet.
	//           Soporta Vanilla, Paper, Spigot, Forge, Fabric, etc.
	//
	// - "bedrock": Usa UDP/RakNet. Cada servidor obtiene un puerto dedicado
	//             del rango configurado. Soporta Vanilla Bedrock, PocketMine, etc.
	//
	// +kubebuilder:validation:Required
	Edition Edition `json:"edition"`

	// Hostname es el subdominio completo que el jugador usa para conectarse.
	// Ejemplo: "jugador123.miminecraftserver.com"
	//
	// Para Java Edition: se usa para routing basado en el handshake packet.
	// Para Bedrock Edition: se usa como referencia DNS; el routing real es por puerto.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`
	Hostname string `json:"hostname"`

	// Backend define el Service de Kubernetes al que se redirige el tráfico.
	// +kubebuilder:validation:Required
	Backend BackendSpec `json:"backend"`

	// MaxPlayers es el número máximo de conexiones simultáneas permitidas.
	// 0 = sin límite.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	MaxPlayers int32 `json:"maxPlayers,omitempty"`

	// AssignedPort es el puerto externo asignado a este servidor.
	//
	// Para Java Edition: se ignora (todos comparten el puerto 25565 y se rutea por hostname).
	// Para Bedrock Edition: es OBLIGATORIO. Cada servidor necesita un puerto UDP dedicado
	//                       en el rango 19132-29132.
	//
	// Si no se especifica para Bedrock, el operator lo asigna automáticamente del rango disponible.
	//
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	AssignedPort int32 `json:"assignedPort,omitempty"`

	// RateLimit define límites de conexión por IP de origen.
	// +optional
	RateLimit *RateLimitSpec `json:"rateLimit,omitempty"`
}

// BackendSpec define el Service destino dentro del clúster.
type BackendSpec struct {
	// ServiceName es el nombre del Service de Kubernetes que expone el servidor Minecraft.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`

	// ServicePort es el puerto del Service.
	// Java Edition default: 25565
	// Bedrock Edition default: 19132
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ServicePort int32 `json:"servicePort,omitempty"`

	// Namespace es el namespace donde se encuentra el Service.
	// Si se omite, se usa el mismo namespace del MinecraftProxy CR.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// RateLimitSpec define la configuración de rate limiting.
type RateLimitSpec struct {
	// ConnectionsPerMinute es el máximo de nuevas conexiones por IP por minuto.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	ConnectionsPerMinute int32 `json:"connectionsPerMinute,omitempty"`
}

// MinecraftProxyStatus define el estado observado del proxy.
type MinecraftProxyStatus struct {
	// Ready indica si la ruta está activa en el proxy.
	// +kubebuilder:default=false
	Ready bool `json:"ready,omitempty"`

	// ActiveConnections es el número de conexiones activas hacia este backend.
	ActiveConnections int32 `json:"activeConnections,omitempty"`

	// AssignedPort es el puerto realmente asignado (relevante para Bedrock).
	// Este campo lo rellena el operator cuando auto-asigna un puerto.
	// +optional
	AssignedPort int32 `json:"assignedPort,omitempty"`

	// Edition refleja la edición configurada para confirmación visual.
	// +optional
	Edition Edition `json:"edition,omitempty"`

	// LastConnected es el timestamp de la última conexión recibida.
	// +optional
	LastConnected *metav1.Time `json:"lastConnected,omitempty"`

	// Conditions representa las condiciones actuales del recurso.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Edition",type=string,JSONPath=`.spec.edition`
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.hostname`
// +kubebuilder:printcolumn:name="Backend",type=string,JSONPath=`.spec.backend.serviceName`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.status.assignedPort`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Connections",type=integer,JSONPath=`.status.activeConnections`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MinecraftProxy es el Schema para el API de minecraftproxies
type MinecraftProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MinecraftProxySpec   `json:"spec,omitempty"`
	Status MinecraftProxyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MinecraftProxyList contiene una lista de MinecraftProxy
type MinecraftProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MinecraftProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MinecraftProxy{}, &MinecraftProxyList{})
}

// DefaultServicePort retorna el puerto por defecto según la edición.
func DefaultServicePort(edition Edition) int32 {
	switch edition {
	case EditionBedrock:
		return 19132
	default:
		return 25565
	}
}
```

### 2.2 Regenerar manifests

```bash
make generate
make manifests
```

### 2.3 Ejemplo de CR Java: `config/samples/minecraft_v1alpha1_java.yaml`

```yaml
apiVersion: minecraft.miminecraftserver.com/v1alpha1
kind: MinecraftProxy
metadata:
  name: jugador1-java
  namespace: minecraft-system
spec:
  edition: java
  hostname: "jugador1.miminecraftserver.com"
  backend:
    serviceName: "jugador1-minecraft"
    servicePort: 25565
    namespace: "mc-servers"
  maxPlayers: 20
  rateLimit:
    connectionsPerMinute: 10
```

### 2.4 Ejemplo de CR Bedrock: `config/samples/minecraft_v1alpha1_bedrock.yaml`

```yaml
apiVersion: minecraft.miminecraftserver.com/v1alpha1
kind: MinecraftProxy
metadata:
  name: jugador2-bedrock
  namespace: minecraft-system
spec:
  edition: bedrock
  hostname: "jugador2.miminecraftserver.com"
  backend:
    serviceName: "jugador2-minecraft-bedrock"
    servicePort: 19132
    namespace: "mc-servers"
  maxPlayers: 10
  # assignedPort se omite: el operator asigna uno automáticamente del rango 19132-29132
```

---

## Paso 3: Interface ProtocolHandler y Registry (Strategy Pattern)

### 3.1 Archivo: `internal/proxy/handler.go`

Este es el archivo central del Strategy Pattern. Define la interfaz que todas las ediciones deben implementar, y el registry que las gestiona.

```go
package proxy

import (
	"context"
	"fmt"
	"sync"
)

// ProtocolHandler es la interfaz que cada edición de Minecraft debe implementar.
// Define el contrato para manejar conexiones de un protocolo específico.
//
// Cada implementación es responsable de:
// 1. Escuchar en su puerto/protocolo correspondiente
// 2. Extraer el identificador de routing (hostname, puerto, etc.)
// 3. Buscar el backend en la RouteTable
// 4. Hacer proxy del tráfico hacia el backend
type ProtocolHandler interface {
	// Name retorna el identificador de la edición.
	// Debe coincidir con el valor del campo `spec.edition` del CRD.
	// Ejemplos: "java", "bedrock"
	Name() string

	// DefaultPort retorna el puerto estándar para esta edición.
	// Java: 25565, Bedrock: 19132
	DefaultPort() int32

	// Start inicia el listener del handler.
	// Debe bloquear hasta que el contexto se cancele.
	// Recibe la RouteTable compartida para hacer lookups de rutas.
	//
	// Para Java: inicia un TCP listener en el puerto configurado.
	// Para Bedrock: gestiona múltiples UDP listeners (uno por servidor).
	Start(ctx context.Context, routeTable *RouteTable) error

	// Stop detiene gracefully el handler.
	// Debe cerrar listeners y esperar a que las conexiones activas drenen.
	Stop() error

	// AddRoute notifica al handler que una nueva ruta fue configurada.
	// Esto es necesario porque algunos handlers (Bedrock) necesitan
	// abrir un listener nuevo por cada ruta.
	//
	// Para Java: no-op (el único listener TCP maneja todas las rutas).
	// Para Bedrock: abre un nuevo UDP listener en el puerto asignado.
	//
	// Parámetros:
	// - hostname: el hostname del CRD
	// - backend: el backend destino
	// - assignedPort: el puerto externo asignado (relevante para Bedrock)
	AddRoute(hostname string, backend *Backend, assignedPort int32) error

	// RemoveRoute notifica al handler que una ruta fue eliminada.
	//
	// Para Java: no-op (la route table se actualiza directamente).
	// Para Bedrock: cierra el UDP listener del puerto asignado.
	RemoveRoute(hostname string, assignedPort int32) error
}

// HandlerRegistry almacena y gestiona los ProtocolHandlers disponibles.
// Es thread-safe para permitir consultas desde el reconciler.
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ProtocolHandler
}

// NewHandlerRegistry crea un registry vacío.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]ProtocolHandler),
	}
}

// Register registra un handler para una edición.
// Se llama durante la inicialización del operator (en main.go).
// Sobrescribe si ya existe un handler con el mismo nombre.
func (r *HandlerRegistry) Register(handler ProtocolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[handler.Name()] = handler
}

// Get retorna el handler para una edición dada.
// Retorna el handler y un bool indicando si se encontró.
func (r *HandlerRegistry) Get(edition string) (ProtocolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[edition]
	return h, ok
}

// ListEditions retorna todas las ediciones registradas.
func (r *HandlerRegistry) ListEditions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	editions := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		editions = append(editions, name)
	}
	return editions
}

// StartAll inicia todos los handlers registrados en goroutines separadas.
// Retorna un error si alguno falla al iniciar.
// Cada handler se ejecuta en su propia goroutine.
func (r *HandlerRegistry) StartAll(ctx context.Context, routeTable *RouteTable) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	errCh := make(chan error, len(r.handlers))

	for _, handler := range r.handlers {
		h := handler // capture loop variable
		go func() {
			if err := h.Start(ctx, routeTable); err != nil {
				errCh <- fmt.Errorf("handler %s: %w", h.Name(), err)
			}
		}()
	}

	// Nota: Start() bloquea, así que los errores llegan cuando un handler
	// termina inesperadamente. El caller (main.go) debe monitorear errCh.
	// Para simplificar, retornamos nil aquí y el manejo de errores
	// se hace en main.go observando el contexto.
	return nil
}

// StopAll detiene todos los handlers.
func (r *HandlerRegistry) StopAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastErr error
	for _, handler := range r.handlers {
		if err := handler.Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
```

### 3.2 Archivo: `internal/proxy/handler_test.go`

```go
package proxy

import "testing"

// TestHandlerRegistry_RegisterAndGet
// Registrar un mock handler con Name()="test".
// Get("test") debe retornar el handler y ok=true.
// Get("noexiste") debe retornar ok=false.

// TestHandlerRegistry_ListEditions
// Registrar handlers "java" y "bedrock".
// ListEditions() debe retornar ambos (orden no importa).

// TestHandlerRegistry_RegisterOverwrites
// Registrar handler "java" con mock A.
// Registrar handler "java" con mock B.
// Get("java") debe retornar mock B.

// --- Mock handler para tests ---
// type mockHandler struct {
//     name        string
//     defaultPort int32
//     started     bool
// }
// Implementar todos los métodos de ProtocolHandler como no-ops.
```

---

## Paso 4: Tabla de Rutas (Route Table)

### 4.1 Archivo: `internal/proxy/routetable.go`

La route table ahora necesita manejar rutas de diferentes ediciones. La key cambia dependiendo de la edición:

- **Java**: key = hostname (porque el routing es por hostname del handshake)
- **Bedrock**: key = puerto asignado (porque el routing es por puerto UDP)

Se usan dos mapas internos para evitar conflictos entre ediciones.

```go
package proxy

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Backend representa un destino de proxy dentro del clúster de Kubernetes.
type Backend struct {
	// ServiceName es el nombre del Service de Kubernetes.
	ServiceName string

	// ServicePort es el puerto del Service.
	ServicePort int32

	// Namespace es el namespace del Service.
	Namespace string

	// MaxPlayers es el máximo de conexiones simultáneas (0 = sin límite).
	MaxPlayers int32

	// Edition es la edición de Minecraft para este backend.
	Edition string

	// AssignedPort es el puerto externo (relevante para Bedrock).
	AssignedPort int32

	// Hostname es el hostname original del CRD.
	Hostname string

	// ActiveConnections lleva la cuenta de conexiones activas (atómico).
	ActiveConnections atomic.Int32
}

// Address retorna la dirección DNS de Kubernetes del Service:
// {serviceName}.{namespace}.svc.cluster.local:{port}
func (b *Backend) Address() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", b.ServiceName, b.Namespace, b.ServicePort)
}

// RouteTable gestiona las rutas para todas las ediciones.
// Usa mapas separados para hostname-based routing (Java) y port-based routing (Bedrock).
//
// Thread-safe: usa RWMutex para lecturas concurrentes desde los handlers
// y escrituras exclusivas desde el reconciler.
type RouteTable struct {
	mu sync.RWMutex

	// hostnameRoutes mapea hostname → Backend (para Java Edition)
	// Key: hostname en minúsculas
	hostnameRoutes map[string]*Backend

	// portRoutes mapea puerto → Backend (para Bedrock Edition)
	// Key: puerto asignado
	portRoutes map[int32]*Backend
}

// NewRouteTable crea una nueva RouteTable vacía.
func NewRouteTable() *RouteTable {
	return &RouteTable{
		hostnameRoutes: make(map[string]*Backend),
		portRoutes:     make(map[int32]*Backend),
	}
}

// --- Operaciones para Java Edition (hostname-based) ---

// SetHostnameRoute agrega o actualiza una ruta basada en hostname.
// Usado por Java Edition.
func (rt *RouteTable) SetHostnameRoute(hostname string, backend *Backend) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.hostnameRoutes[strings.ToLower(hostname)] = backend
}

// GetHostnameRoute busca un backend por hostname.
// Usado por Java Edition durante el routing.
func (rt *RouteTable) GetHostnameRoute(hostname string) (*Backend, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	backend, ok := rt.hostnameRoutes[strings.ToLower(hostname)]
	return backend, ok
}

// RemoveHostnameRoute elimina una ruta por hostname.
func (rt *RouteTable) RemoveHostnameRoute(hostname string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.hostnameRoutes, strings.ToLower(hostname))
}

// --- Operaciones para Bedrock Edition (port-based) ---

// SetPortRoute agrega o actualiza una ruta basada en puerto.
// Usado por Bedrock Edition.
func (rt *RouteTable) SetPortRoute(port int32, backend *Backend) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.portRoutes[port] = backend
}

// GetPortRoute busca un backend por puerto asignado.
// Usado por Bedrock Edition durante el routing.
func (rt *RouteTable) GetPortRoute(port int32) (*Backend, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	backend, ok := rt.portRoutes[port]
	return backend, ok
}

// RemovePortRoute elimina una ruta por puerto.
func (rt *RouteTable) RemovePortRoute(port int32) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.portRoutes, port)
}

// --- Operaciones generales ---

// HostnameCount retorna el número de rutas hostname (Java).
func (rt *RouteTable) HostnameCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.hostnameRoutes)
}

// PortCount retorna el número de rutas por puerto (Bedrock).
func (rt *RouteTable) PortCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.portRoutes)
}

// ListHostnames retorna todos los hostnames registrados.
func (rt *RouteTable) ListHostnames() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	hostnames := make([]string, 0, len(rt.hostnameRoutes))
	for h := range rt.hostnameRoutes {
		hostnames = append(hostnames, h)
	}
	return hostnames
}

// ListPorts retorna todos los puertos registrados.
func (rt *RouteTable) ListPorts() []int32 {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	ports := make([]int32, 0, len(rt.portRoutes))
	for p := range rt.portRoutes {
		ports = append(ports, p)
	}
	return ports
}

// IsPortInUse verifica si un puerto ya está asignado.
func (rt *RouteTable) IsPortInUse(port int32) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	_, ok := rt.portRoutes[port]
	return ok
}
```

### 4.2 Archivo: `internal/proxy/routetable_test.go`

```go
package proxy

import "testing"

// TestRouteTable_HostnameRoutes_SetGetRemove
// Verificar Set, Get, Remove para rutas por hostname.
// Get de hostname inexistente retorna ok=false.

// TestRouteTable_HostnameRoutes_CaseInsensitive
// Set("HOST.Example.COM") → Get("host.example.com") == ok

// TestRouteTable_PortRoutes_SetGetRemove
// Verificar Set, Get, Remove para rutas por puerto.
// Get de puerto inexistente retorna ok=false.

// TestRouteTable_HostnameAndPortIndependent
// Verificar que hostname routes y port routes no interfieren.
// Set hostname route, Set port route, ambos deben existir independientemente.

// TestRouteTable_IsPortInUse
// Puerto no asignado → false. Asignar → true. Remover → false.

// TestRouteTable_ConcurrentAccess
// Múltiples goroutines leyendo y escribiendo simultáneamente.
// Ejecutar con -race para detectar data races.
```

---

## Paso 5: Port Manager para Bedrock

### 5.1 Archivo: `internal/proxy/portmanager/manager.go`

Gestiona la asignación automática de puertos UDP para servidores Bedrock.

```go
package portmanager

import (
	"fmt"
	"sync"
)

const (
	// DefaultMinPort es el inicio del rango de puertos para Bedrock
	DefaultMinPort int32 = 19132

	// DefaultMaxPort es el final del rango de puertos para Bedrock
	DefaultMaxPort int32 = 29132
)

// PortManager gestiona la asignación de puertos UDP para Bedrock Edition.
// Mantiene un registro de puertos en uso y asigna puertos disponibles
// del rango configurado.
type PortManager struct {
	mu       sync.Mutex
	minPort  int32
	maxPort  int32
	usedPorts map[int32]string // port → hostname (para tracking)
}

// NewPortManager crea un PortManager con el rango especificado.
func NewPortManager(minPort, maxPort int32) *PortManager {
	return &PortManager{
		minPort:   minPort,
		maxPort:   maxPort,
		usedPorts: make(map[int32]string),
	}
}

// Allocate busca y asigna el siguiente puerto disponible.
// Retorna el puerto asignado o error si no hay puertos disponibles.
//
// El hostname se usa para tracking (saber a quién pertenece el puerto).
//
// Algoritmo: scan lineal desde minPort hasta maxPort buscando el primer
// puerto libre. Para el volumen esperado (<1000 servidores) esto es eficiente.
func (pm *PortManager) Allocate(hostname string) (int32, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.minPort; port <= pm.maxPort; port++ {
		if _, used := pm.usedPorts[port]; !used {
			pm.usedPorts[port] = hostname
			return port, nil
		}
	}
	return 0, fmt.Errorf("no hay puertos disponibles en el rango %d-%d", pm.minPort, pm.maxPort)
}

// AllocateSpecific intenta asignar un puerto específico.
// Retorna error si el puerto ya está en uso o fuera de rango.
//
// Se usa cuando el CRD ya tiene un assignedPort definido.
func (pm *PortManager) AllocateSpecific(port int32, hostname string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if port < pm.minPort || port > pm.maxPort {
		return fmt.Errorf("puerto %d fuera del rango permitido (%d-%d)", port, pm.minPort, pm.maxPort)
	}

	if existingHost, used := pm.usedPorts[port]; used {
		return fmt.Errorf("puerto %d ya asignado a %s", port, existingHost)
	}

	pm.usedPorts[port] = hostname
	return nil
}

// Release libera un puerto previamente asignado.
func (pm *PortManager) Release(port int32) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.usedPorts, port)
}

// IsUsed verifica si un puerto está en uso.
func (pm *PortManager) IsUsed(port int32) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	_, used := pm.usedPorts[port]
	return used
}

// UsedCount retorna el número de puertos en uso.
func (pm *PortManager) UsedCount() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return len(pm.usedPorts)
}

// AvailableCount retorna el número de puertos disponibles.
func (pm *PortManager) AvailableCount() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	total := pm.maxPort - pm.minPort + 1
	return int(total) - len(pm.usedPorts)
}
```

### 5.2 Archivo: `internal/proxy/portmanager/manager_test.go`

```go
package portmanager

import "testing"

// TestPortManager_Allocate
// Crear manager con rango 19132-19135 (4 puertos).
// Allocate 4 veces → debe retornar 19132, 19133, 19134, 19135.
// Quinto Allocate → debe retornar error "no hay puertos disponibles".

// TestPortManager_AllocateSpecific
// AllocateSpecific(19133, "host1") → ok.
// AllocateSpecific(19133, "host2") → error "ya asignado a host1".
// AllocateSpecific(99999, "host3") → error "fuera de rango".

// TestPortManager_Release
// Allocate → Release → el puerto vuelve a estar disponible.

// TestPortManager_AllocateAfterRelease
// Llenar todos los puertos. Release uno. Allocate → debe dar el liberado.

// TestPortManager_ConcurrentAllocations
// Múltiples goroutines allocando simultáneamente. No debe haber duplicados.
```

---

## Paso 6: Java Protocol Handler

### 6.1 Archivo: `internal/proxy/java/minecraft.go`

Parser del Handshake Packet de Minecraft Java Edition.

```go
package java

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Constantes del protocolo Java Edition
const (
	MaxVarIntBytes         = 5
	MaxHandshakeSize       = 512
	MaxServerAddressLength = 255
	HandshakePacketID      = 0x00
	StateStatus            = 1
	StateLogin             = 2
)

// Handshake representa un paquete de handshake parseado.
type Handshake struct {
	ProtocolVersion int32
	ServerAddress   string // Hostname limpio (sin sufijos FML, en minúsculas)
	ServerPort      uint16
	NextState       int32
}

// ReadHandshake lee y parsea el handshake de un io.Reader.
//
// Retorna:
// - *Handshake: la estructura parseada
// - []byte: los bytes raw originales (length prefix + payload) para reenviar al backend
// - error: si el paquete es inválido o hay un error de lectura
//
// Flujo:
// 1. Leer packet length (VarInt), guardando bytes raw
// 2. Leer exactamente packetLength bytes del payload
// 3. Parsear payload: packet ID (0x00), protocol version, server address, port, next state
// 4. Limpiar server address (quitar FML suffixes, lowercase)
// 5. Retornar estructura + raw bytes
func ReadHandshake(reader io.Reader) (*Handshake, []byte, error) {
	// Paso 1: Leer packet length
	packetLength, lengthBytes, err := readVarIntRaw(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo packet length: %w", err)
	}
	if packetLength <= 0 || packetLength > MaxHandshakeSize {
		return nil, nil, fmt.Errorf("packet length inválido: %d", packetLength)
	}

	// Paso 2: Leer payload completo
	payload := make([]byte, packetLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, nil, fmt.Errorf("leyendo payload: %w", err)
	}

	// Raw bytes = length prefix + payload
	rawBytes := append(lengthBytes, payload...)

	// Paso 3: Parsear payload
	offset := 0

	// Packet ID
	packetID, n, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo packet ID: %w", err)
	}
	offset += n
	if packetID != HandshakePacketID {
		return nil, nil, fmt.Errorf("packet ID inesperado: 0x%02X", packetID)
	}

	// Protocol Version
	protocolVersion, n, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo protocol version: %w", err)
	}
	offset += n

	// Server Address
	serverAddress, n, err := readStringFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo server address: %w", err)
	}
	offset += n

	// Server Port (uint16 Big Endian)
	if offset+2 > len(payload) {
		return nil, nil, errors.New("payload truncado en server port")
	}
	serverPort := binary.BigEndian.Uint16(payload[offset : offset+2])
	offset += 2

	// Next State
	nextState, _, err := readVarIntFromBytes(payload, offset)
	if err != nil {
		return nil, nil, fmt.Errorf("leyendo next state: %w", err)
	}

	return &Handshake{
		ProtocolVersion: protocolVersion,
		ServerAddress:   cleanServerAddress(serverAddress),
		ServerPort:      serverPort,
		NextState:       nextState,
	}, rawBytes, nil
}

// cleanServerAddress remueve sufijos FML de Forge y normaliza a minúsculas.
//
// Forge agrega al hostname:
// - "\x00FML\x00"   (Forge legacy)
// - "\x00FML2\x00"  (Forge nuevo)
// - "\x00FML3\x00"  (Forge moderno)
//
// Se corta en el primer null byte y se convierte a lowercase.
func cleanServerAddress(address string) string {
	if idx := strings.IndexByte(address, 0x00); idx != -1 {
		address = address[:idx]
	}
	return strings.ToLower(strings.TrimSpace(address))
}

// --- Funciones auxiliares VarInt/String ---

// readVarIntRaw lee un VarInt de un io.Reader.
// Retorna: valor, bytes raw leídos, error.
func readVarIntRaw(reader io.Reader) (int32, []byte, error) {
	var result int32
	var numRead uint
	var rawBytes []byte
	buf := make([]byte, 1)

	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			return 0, nil, err
		}
		rawBytes = append(rawBytes, buf[0])
		result |= int32(buf[0]&0x7F) << (7 * numRead)
		numRead++
		if numRead > MaxVarIntBytes {
			return 0, nil, errors.New("VarInt demasiado grande")
		}
		if buf[0]&0x80 == 0 {
			break
		}
	}
	return result, rawBytes, nil
}

// readVarIntFromBytes lee un VarInt de un byte slice.
// Retorna: valor, bytes consumidos, error.
func readVarIntFromBytes(data []byte, offset int) (int32, int, error) {
	var result int32
	var numRead int

	for {
		if offset+numRead >= len(data) {
			return 0, 0, errors.New("VarInt truncado")
		}
		b := data[offset+numRead]
		result |= int32(b&0x7F) << (7 * numRead)
		numRead++
		if numRead > MaxVarIntBytes {
			return 0, 0, errors.New("VarInt demasiado grande")
		}
		if b&0x80 == 0 {
			break
		}
	}
	return result, numRead, nil
}

// readStringFromBytes lee un MC String (VarInt length + UTF-8 bytes).
// Retorna: string, total bytes consumidos, error.
func readStringFromBytes(data []byte, offset int) (string, int, error) {
	strLen, n, err := readVarIntFromBytes(data, offset)
	if err != nil {
		return "", 0, err
	}
	if strLen < 0 || strLen > MaxServerAddressLength {
		return "", 0, fmt.Errorf("longitud de string inválida: %d", strLen)
	}
	start := offset + n
	end := start + int(strLen)
	if end > len(data) {
		return "", 0, errors.New("string truncado")
	}
	return string(data[start:end]), n + int(strLen), nil
}
```

### 6.2 Archivo: `internal/proxy/java/minecraft_test.go`

```go
package java

import (
	"bytes"
	"testing"
)

// Helper: buildHandshakePacket construye un paquete binario completo.
// func buildHandshakePacket(protocolVersion int32, serverAddress string, serverPort uint16, nextState int32) []byte
//
// Implementar usando writeVarInt y binary.BigEndian.PutUint16.
// Estructura: VarInt(payloadLen) + VarInt(0x00) + VarInt(protoVer) + MCString(addr) + uint16(port) + VarInt(state)

// TestReadHandshake_ValidLogin
// Packet: protocol=765, address="jugador1.example.com", port=25565, state=2
// Verificar todos los campos. Verificar que rawBytes reconstruyen el paquete.

// TestReadHandshake_ValidStatus
// Packet con state=1 (Server List Ping).

// TestReadHandshake_ForgeClient
// Address: "jugador1.example.com\x00FML\x00"
// Resultado: ServerAddress == "jugador1.example.com" (sin FML)

// TestReadHandshake_FML2Client
// Address: "jugador1.example.com\x00FML2\x00"

// TestReadHandshake_CaseInsensitive
// Address: "Jugador1.EXAMPLE.Com" → "jugador1.example.com"

// TestReadHandshake_InvalidPacketID
// Construir paquete con packet ID 0x01 → error.

// TestReadHandshake_TruncatedPayload
// Enviar solo los primeros 5 bytes de un paquete válido → error.

// TestReadHandshake_OversizedPacket
// Packet con length > MaxHandshakeSize → error.

// TestReadHandshake_RawBytesAreCorrect
// Verificar que rawBytes == el paquete original byte-a-byte.
// Esto es crítico porque los raw bytes se reenvían al backend.
```

### 6.3 Archivo: `internal/proxy/java/handler.go`

```go
package java

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/tuuser/mc-proxy-operator/internal/proxy"
)

const (
	// DefaultListenAddr es la dirección TCP para Java Edition
	DefaultListenAddr = ":25565"

	// HandshakeTimeout es el tiempo máximo para leer el handshake
	HandshakeTimeout = 10 * time.Second

	// DialTimeout es el tiempo máximo para conectar al backend
	DialTimeout = 5 * time.Second
)

// JavaProtocolHandler implementa proxy.ProtocolHandler para Minecraft Java Edition.
//
// Funciona como un TCP reverse proxy que:
// 1. Escucha en un puerto TCP (default 25565)
// 2. Lee el Handshake Packet de cada conexión entrante
// 3. Extrae el hostname del handshake
// 4. Busca el backend en la RouteTable (por hostname)
// 5. Reenvía los bytes raw del handshake al backend
// 6. Hace proxy TCP bidireccional (io.Copy en ambas direcciones)
type JavaProtocolHandler struct {
	listenAddr string
	listener   net.Listener
	routeTable *proxy.RouteTable
	wg         sync.WaitGroup
}

// NewJavaProtocolHandler crea una nueva instancia.
// listenAddr es la dirección TCP (ej: ":25565").
func NewJavaProtocolHandler(listenAddr string) *JavaProtocolHandler {
	return &JavaProtocolHandler{
		listenAddr: listenAddr,
	}
}

// Name retorna "java" — debe coincidir con el valor del enum Edition en el CRD.
func (h *JavaProtocolHandler) Name() string {
	return "java"
}

// DefaultPort retorna 25565.
func (h *JavaProtocolHandler) DefaultPort() int32 {
	return 25565
}

// Start inicia el TCP listener y el loop de accept.
// Bloquea hasta que el contexto se cancele.
//
// Flujo:
// 1. net.Listen("tcp", listenAddr)
// 2. Goroutine: cuando ctx se cancele → listener.Close()
// 3. Loop: Accept() → goroutine handleConnection()
// 4. Cuando el listener se cierra, salir del loop
// 5. wg.Wait() para esperar conexiones activas
func (h *JavaProtocolHandler) Start(ctx context.Context, routeTable *proxy.RouteTable) error {
	h.routeTable = routeTable
	logger := log.FromContext(ctx).WithValues("handler", "java")

	var err error
	h.listener, err = net.Listen("tcp", h.listenAddr)
	if err != nil {
		return err
	}
	logger.Info("Java handler escuchando", "addr", h.listenAddr)

	// Cerrar listener cuando el contexto se cancele
	go func() {
		<-ctx.Done()
		h.listener.Close()
	}()

	// Accept loop
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // Shutdown graceful
			}
			logger.Error(err, "error aceptando conexión")
			continue
		}
		h.wg.Add(1)
		go h.handleConnection(ctx, conn)
	}

	// Esperar conexiones activas
	h.wg.Wait()
	logger.Info("Java handler detenido")
	return nil
}

// Stop cierra el listener TCP.
func (h *JavaProtocolHandler) Stop() error {
	if h.listener != nil {
		return h.listener.Close()
	}
	return nil
}

// AddRoute es un no-op para Java Edition.
// Java usa un solo listener TCP y rutea por hostname en el handshake,
// así que no necesita abrir listeners nuevos por cada ruta.
func (h *JavaProtocolHandler) AddRoute(hostname string, backend *proxy.Backend, assignedPort int32) error {
	return nil
}

// RemoveRoute es un no-op para Java Edition.
func (h *JavaProtocolHandler) RemoveRoute(hostname string, assignedPort int32) error {
	return nil
}

// handleConnection maneja una conexión TCP individual.
//
// Flujo:
// 1. Set deadline para handshake (HandshakeTimeout)
// 2. ReadHandshake() → hostname + raw bytes
// 3. RouteTable.GetHostnameRoute(hostname)
//    - No encontrado → log + cerrar conexión
// 4. Verificar MaxPlayers
//    - Si activeConnections >= maxPlayers → log + cerrar
// 5. net.DialTimeout("tcp", backend.Address(), DialTimeout)
//    - Error → log + cerrar
// 6. backend.ActiveConnections.Add(1); defer Add(-1)
// 7. Escribir rawBytes al backend (reenviar handshake)
// 8. Limpiar deadline del cliente
// 9. Proxy bidireccional:
//    - goroutine: io.Copy(backend, client) → client-to-backend
//    - goroutine: io.Copy(client, backend) → backend-to-client
//    - Cuando cualquiera termina, cerrar ambas conexiones con CloseWrite
// 10. Log de desconexión con duración y bytes
func (h *JavaProtocolHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer h.wg.Done()
	defer clientConn.Close()

	logger := log.FromContext(ctx).WithValues("handler", "java")
	remoteAddr := clientConn.RemoteAddr().String()

	// 1. Deadline
	clientConn.SetDeadline(time.Now().Add(HandshakeTimeout))

	// 2. Leer handshake
	handshake, rawBytes, err := ReadHandshake(clientConn)
	if err != nil {
		logger.V(1).Info("handshake inválido", "remote", remoteAddr, "error", err)
		return
	}

	logger.Info("handshake recibido",
		"remote", remoteAddr,
		"hostname", handshake.ServerAddress,
		"protocol", handshake.ProtocolVersion,
		"state", handshake.NextState,
	)

	// 3. Buscar ruta
	backend, found := h.routeTable.GetHostnameRoute(handshake.ServerAddress)
	if !found {
		logger.Info("ruta no encontrada", "hostname", handshake.ServerAddress)
		return
	}

	// 4. Verificar MaxPlayers
	if backend.MaxPlayers > 0 && backend.ActiveConnections.Load() >= backend.MaxPlayers {
		logger.Info("servidor lleno", "hostname", handshake.ServerAddress)
		return
	}

	// 5. Conectar al backend
	backendAddr := backend.Address()
	backendConn, err := net.DialTimeout("tcp", backendAddr, DialTimeout)
	if err != nil {
		logger.Error(err, "error conectando al backend", "backend", backendAddr)
		return
	}
	defer backendConn.Close()

	// 6. Track conexión
	backend.ActiveConnections.Add(1)
	defer backend.ActiveConnections.Add(-1)

	// 7. Reenviar handshake
	if _, err := backendConn.Write(rawBytes); err != nil {
		logger.Error(err, "error reenviando handshake")
		return
	}

	// 8. Limpiar deadline
	clientConn.SetDeadline(time.Time{})

	logger.Info("proxy establecido", "remote", remoteAddr, "hostname", handshake.ServerAddress, "backend", backendAddr)

	// 9. Proxy bidireccional
	startTime := time.Now()
	errCh := make(chan error, 2)
	var c2b, b2c int64

	go func() {
		n, err := io.Copy(backendConn, clientConn)
		c2b = n
		if tc, ok := backendConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	go func() {
		n, err := io.Copy(clientConn, backendConn)
		b2c = n
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		errCh <- err
	}()

	<-errCh
	<-errCh

	// 10. Log
	logger.Info("conexión cerrada",
		"remote", remoteAddr,
		"hostname", handshake.ServerAddress,
		"duration", time.Since(startTime).String(),
		"clientToBackend", c2b,
		"backendToClient", b2c,
	)
}
```

---

## Paso 7: Bedrock Protocol Handler

### 7.1 Archivo: `internal/proxy/bedrock/raknet.go`

Parser básico de paquetes RakNet para Bedrock Edition.

```go
package bedrock

// Constantes del protocolo RakNet/Bedrock
const (
	// IDUnconnectedPing es el tipo de paquete de ping inicial del cliente Bedrock.
	// El cliente envía esto como primer paquete para descubrir servidores.
	IDUnconnectedPing = 0x01

	// IDUnconnectedPong es la respuesta del servidor al ping.
	IDUnconnectedPong = 0x1C

	// RakNetMagic son los 16 bytes mágicos que identifican paquetes RakNet offline.
	// Presentes en Unconnected Ping/Pong.
	// Valor: 00 ff ff 00 fe fe fe fe fd fd fd fd 12 34 56 78
)

// RakNetMagic es la secuencia mágica de 16 bytes del protocolo RakNet.
var RakNetMagic = []byte{
	0x00, 0xff, 0xff, 0x00,
	0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd,
	0x12, 0x34, 0x56, 0x78,
}

// IsRakNetPacket verifica si un paquete UDP es un paquete RakNet offline
// comprobando si contiene la secuencia mágica.
//
// Los paquetes RakNet offline (Unconnected Ping/Pong) tienen la magia
// en offset 17 (después del packet ID + timestamp + client GUID).
//
// Parámetro: data es el payload UDP completo.
// Retorna: true si es un paquete RakNet offline.
func IsRakNetPacket(data []byte) bool {
	// Verificar longitud mínima y buscar la magia en las posiciones conocidas
	// Para Unconnected Ping: ID(1) + Time(8) + Magic(16) + ClientGUID(8) = 33 bytes mínimo
	if len(data) < 33 {
		return false
	}

	// La magia empieza en offset 9 para Unconnected Ping
	if data[0] == IDUnconnectedPing {
		return bytesEqual(data[9:25], RakNetMagic)
	}
	return false
}

// bytesEqual compara dos slices de bytes.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

### 7.2 Archivo: `internal/proxy/bedrock/handler.go`

```go
package bedrock

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/tuuser/mc-proxy-operator/internal/proxy"
)

const (
	// UDPBufferSize es el tamaño del buffer para leer paquetes UDP.
	// Los paquetes RakNet pueden ser hasta ~1500 bytes (MTU).
	UDPBufferSize = 2048

	// SessionTimeout es el tiempo de inactividad antes de cerrar una sesión UDP.
	// Bedrock no tiene "conexión" TCP, así que usamos un timeout de inactividad.
	SessionTimeout = 60 * time.Second

	// SessionCleanupInterval es el intervalo para limpiar sesiones expiradas.
	SessionCleanupInterval = 30 * time.Second
)

// udpSession representa una sesión de proxy UDP entre un cliente y un backend.
type udpSession struct {
	// clientAddr es la dirección del cliente Bedrock
	clientAddr *net.UDPAddr

	// backendConn es la conexión UDP al servidor backend
	backendConn *net.UDPConn

	// lastActivity es el timestamp de la última actividad
	lastActivity time.Time

	// backend es el backend asociado
	backend *proxy.Backend
}

// bedrockListener representa un listener UDP para un servidor Bedrock específico.
type bedrockListener struct {
	port     int32
	conn     *net.UDPConn
	hostname string
	backend  *proxy.Backend
	sessions map[string]*udpSession // clientAddr.String() → session
	mu       sync.RWMutex
	cancel   context.CancelFunc
}

// BedrockProtocolHandler implementa proxy.ProtocolHandler para Minecraft Bedrock Edition.
//
// A diferencia de Java, Bedrock usa UDP y no envía hostname en el paquete inicial.
// Por esto, cada servidor Bedrock obtiene su propio puerto UDP dedicado.
//
// El handler mantiene un listener UDP por cada servidor Bedrock activo.
// Cuando se agrega una ruta (AddRoute), se abre un nuevo listener.
// Cuando se elimina (RemoveRoute), se cierra el listener correspondiente.
//
// Cada listener:
// 1. Recibe paquetes UDP del cliente en su puerto asignado
// 2. Mantiene una tabla de sesiones (clientAddr → backendConn)
// 3. Reenvía paquetes cliente→backend y backend→cliente
type BedrockProtocolHandler struct {
	mu         sync.RWMutex
	listeners  map[int32]*bedrockListener // port → listener
	routeTable *proxy.RouteTable
}

// NewBedrockProtocolHandler crea una nueva instancia.
func NewBedrockProtocolHandler() *BedrockProtocolHandler {
	return &BedrockProtocolHandler{
		listeners: make(map[int32]*bedrockListener),
	}
}

// Name retorna "bedrock".
func (h *BedrockProtocolHandler) Name() string {
	return "bedrock"
}

// DefaultPort retorna 19132.
func (h *BedrockProtocolHandler) DefaultPort() int32 {
	return 19132
}

// Start guarda la referencia a la route table y bloquea hasta que el contexto se cancele.
// Los listeners reales se crean dinámicamente con AddRoute.
//
// También inicia un goroutine de limpieza de sesiones expiradas.
func (h *BedrockProtocolHandler) Start(ctx context.Context, routeTable *proxy.RouteTable) error {
	h.routeTable = routeTable
	logger := log.FromContext(ctx).WithValues("handler", "bedrock")
	logger.Info("Bedrock handler iniciado (listeners se crean dinámicamente)")

	// Goroutine de limpieza de sesiones
	go h.cleanupLoop(ctx)

	// Bloquear hasta shutdown
	<-ctx.Done()
	logger.Info("Bedrock handler detenido")
	return nil
}

// Stop cierra todos los listeners UDP activos.
func (h *BedrockProtocolHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	for port, listener := range h.listeners {
		listener.cancel()
		listener.conn.Close()
		delete(h.listeners, port)
	}
	return nil
}

// AddRoute abre un nuevo listener UDP en el puerto asignado.
//
// Flujo:
// 1. Verificar que no exista ya un listener en ese puerto
// 2. Crear UDPConn en ":assignedPort"
// 3. Crear contexto con cancel para este listener
// 4. Iniciar goroutine de lectura de paquetes (readLoop)
// 5. Guardar referencia en h.listeners
func (h *BedrockProtocolHandler) AddRoute(hostname string, backend *proxy.Backend, assignedPort int32) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.listeners[assignedPort]; exists {
		// Ya existe, actualizar backend
		h.listeners[assignedPort].backend = backend
		h.listeners[assignedPort].hostname = hostname
		return nil
	}

	addr := &net.UDPAddr{Port: int(assignedPort)}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("error abriendo UDP en puerto %d: %w", assignedPort, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	listener := &bedrockListener{
		port:     assignedPort,
		conn:     conn,
		hostname: hostname,
		backend:  backend,
		sessions: make(map[string]*udpSession),
		cancel:   cancel,
	}

	h.listeners[assignedPort] = listener

	// Iniciar read loop en goroutine
	go h.readLoop(ctx, listener)

	return nil
}

// RemoveRoute cierra el listener UDP del puerto especificado.
func (h *BedrockProtocolHandler) RemoveRoute(hostname string, assignedPort int32) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	listener, exists := h.listeners[assignedPort]
	if !exists {
		return nil
	}

	listener.cancel()
	listener.conn.Close()

	// Cerrar todas las sesiones
	listener.mu.Lock()
	for _, session := range listener.sessions {
		session.backendConn.Close()
		session.backend.ActiveConnections.Add(-1)
	}
	listener.mu.Unlock()

	delete(h.listeners, assignedPort)
	return nil
}

// readLoop lee paquetes UDP entrantes y los reenvía al backend.
//
// Flujo por cada paquete:
// 1. conn.ReadFromUDP() → data + clientAddr
// 2. Buscar sesión existente para clientAddr
// 3. Si no existe:
//    a. Verificar MaxPlayers
//    b. Crear UDPConn al backend
//    c. Crear sesión nueva
//    d. Iniciar goroutine para leer respuestas del backend (backendReadLoop)
// 4. Actualizar lastActivity
// 5. Reenviar data al backend via session.backendConn
func (h *BedrockProtocolHandler) readLoop(ctx context.Context, listener *bedrockListener) {
	buf := make([]byte, UDPBufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline para poder chequear ctx periódicamente
		listener.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, clientAddr, err := listener.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout normal, chequear ctx
			}
			if ctx.Err() != nil {
				return // Shutdown
			}
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		// Buscar o crear sesión
		session, err := h.getOrCreateSession(listener, clientAddr)
		if err != nil {
			continue // Servidor lleno o error de conexión
		}

		// Actualizar actividad
		session.lastActivity = time.Now()

		// Reenviar al backend
		session.backendConn.Write(data)
	}
}

// getOrCreateSession obtiene una sesión existente o crea una nueva.
func (h *BedrockProtocolHandler) getOrCreateSession(listener *bedrockListener, clientAddr *net.UDPAddr) (*udpSession, error) {
	key := clientAddr.String()

	listener.mu.RLock()
	session, exists := listener.sessions[key]
	listener.mu.RUnlock()

	if exists {
		return session, nil
	}

	// Verificar MaxPlayers
	backend := listener.backend
	if backend.MaxPlayers > 0 && backend.ActiveConnections.Load() >= backend.MaxPlayers {
		return nil, fmt.Errorf("servidor lleno")
	}

	// Conectar al backend
	backendAddr, err := net.ResolveUDPAddr("udp", backend.Address())
	if err != nil {
		return nil, err
	}
	backendConn, err := net.DialUDP("udp", nil, backendAddr)
	if err != nil {
		return nil, err
	}

	session = &udpSession{
		clientAddr:   clientAddr,
		backendConn:  backendConn,
		lastActivity: time.Now(),
		backend:      backend,
	}

	backend.ActiveConnections.Add(1)

	listener.mu.Lock()
	listener.sessions[key] = session
	listener.mu.Unlock()

	// Goroutine para leer respuestas del backend y reenviar al cliente
	go h.backendReadLoop(listener, session)

	return session, nil
}

// backendReadLoop lee paquetes del backend y los reenvía al cliente.
func (h *BedrockProtocolHandler) backendReadLoop(listener *bedrockListener, session *udpSession) {
	buf := make([]byte, UDPBufferSize)

	for {
		session.backendConn.SetReadDeadline(time.Now().Add(SessionTimeout))
		n, err := session.backendConn.Read(buf)
		if err != nil {
			break
		}

		session.lastActivity = time.Now()
		listener.conn.WriteToUDP(buf[:n], session.clientAddr)
	}

	// Limpiar sesión
	listener.mu.Lock()
	delete(listener.sessions, session.clientAddr.String())
	listener.mu.Unlock()

	session.backend.ActiveConnections.Add(-1)
	session.backendConn.Close()
}

// cleanupLoop periódicamente elimina sesiones inactivas.
func (h *BedrockProtocolHandler) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions cierra sesiones que superaron SessionTimeout.
func (h *BedrockProtocolHandler) cleanupExpiredSessions() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	now := time.Now()
	for _, listener := range h.listeners {
		listener.mu.Lock()
		for key, session := range listener.sessions {
			if now.Sub(session.lastActivity) > SessionTimeout {
				session.backendConn.Close()
				session.backend.ActiveConnections.Add(-1)
				delete(listener.sessions, key)
			}
		}
		listener.mu.Unlock()
	}
}
```

### 7.3 Tests para Bedrock

```go
// internal/proxy/bedrock/raknet_test.go

// TestIsRakNetPacket_ValidPing
// Construir un paquete Unconnected Ping con la magia correcta.
// IsRakNetPacket() debe retornar true.

// TestIsRakNetPacket_InvalidMagic
// Paquete con magia incorrecta → false.

// TestIsRakNetPacket_TooShort
// Paquete de 10 bytes → false.

// TestIsRakNetPacket_WrongPacketID
// Paquete con ID 0x05 → false.
```

```go
// internal/proxy/bedrock/handler_test.go

// TestBedrockHandler_AddRemoveRoute
// AddRoute con puerto 19200 → verificar que el listener se creó.
// RemoveRoute → verificar que se cerró.

// TestBedrockHandler_UDPProxy
// 1. Crear fake backend UDP (escucha en puerto local)
// 2. AddRoute apuntando al fake backend
// 3. Enviar paquete UDP al listener del handler
// 4. Verificar que el fake backend recibió el paquete
// 5. Enviar respuesta desde el fake backend
// 6. Verificar que el cliente recibió la respuesta

// TestBedrockHandler_SessionTimeout
// Crear sesión, no enviar más paquetes.
// Esperar > SessionTimeout + cleanup interval.
// Verificar que la sesión fue limpiada.
```

---

## Paso 8: Reconciler Multi-Edition

### 8.1 Archivo: `internal/controller/minecraftproxy_controller.go`

```go
package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	minecraftv1alpha1 "github.com/tuuser/mc-proxy-operator/api/v1alpha1"
	"github.com/tuuser/mc-proxy-operator/internal/proxy"
	"github.com/tuuser/mc-proxy-operator/internal/proxy/portmanager"
)

const finalizerName = "minecraft.miminecraftserver.com/route-cleanup"

// MinecraftProxyReconciler reconcilia objetos MinecraftProxy.
type MinecraftProxyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	RouteTable      *proxy.RouteTable
	HandlerRegistry *proxy.HandlerRegistry
	PortManager     *portmanager.PortManager
}

// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=minecraft.miminecraftserver.com,resources=minecraftproxies/finalizers,verbs=update

// Reconcile es el loop principal del controller.
//
// Lógica completa:
//
// 1. Obtener el CR
// 2. Si no existe (eliminado) → el finalizer ya limpió, retornar
// 3. Manejar eliminación (DeletionTimestamp != 0):
//    a. Obtener el handler de la edición
//    b. Llamar handler.RemoveRoute() para cerrar listeners
//    c. Remover ruta de RouteTable
//    d. Liberar puerto si es Bedrock (PortManager.Release)
//    e. Remover finalizer
// 4. Agregar finalizer si no existe
// 5. Validar que la edición es soportada (handler existe en registry)
// 6. Determinar namespace del backend (default al namespace del CR)
// 7. Determinar service port (default según edición)
// 8. Para Bedrock: asignar puerto si no tiene uno
//    a. Si spec.assignedPort > 0 → intentar AllocateSpecific
//    b. Si no → Allocate automático
//    c. Actualizar spec.assignedPort con el puerto asignado
// 9. Crear Backend struct
// 10. Según la edición:
//     - Java: RouteTable.SetHostnameRoute(hostname, backend)
//     - Bedrock: RouteTable.SetPortRoute(assignedPort, backend)
// 11. Llamar handler.AddRoute() para notificar al handler
// 12. Actualizar status:
//     - Ready = true
//     - Edition = edición
//     - AssignedPort = puerto (Bedrock)
//     - ActiveConnections = valor actual
//     - Condition "RouteConfigured" = True
// 13. Retornar con RequeueAfter 30s (para actualizar connections)
func (r *MinecraftProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Obtener el CR
	var mcProxy minecraftv1alpha1.MinecraftProxy
	if err := r.Get(ctx, req.NamespacedName, &mcProxy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 3. Manejar eliminación
	if !mcProxy.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&mcProxy, finalizerName) {
			edition := string(mcProxy.Spec.Edition)

			// Obtener handler
			if handler, ok := r.HandlerRegistry.Get(edition); ok {
				handler.RemoveRoute(mcProxy.Spec.Hostname, mcProxy.Spec.AssignedPort)
			}

			// Limpiar según edición
			switch mcProxy.Spec.Edition {
			case minecraftv1alpha1.EditionJava:
				r.RouteTable.RemoveHostnameRoute(mcProxy.Spec.Hostname)
			case minecraftv1alpha1.EditionBedrock:
				r.RouteTable.RemovePortRoute(mcProxy.Spec.AssignedPort)
				r.PortManager.Release(mcProxy.Spec.AssignedPort)
			}

			logger.Info("ruta removida",
				"edition", edition,
				"hostname", mcProxy.Spec.Hostname,
				"port", mcProxy.Spec.AssignedPort,
			)

			controllerutil.RemoveFinalizer(&mcProxy, finalizerName)
			if err := r.Update(ctx, &mcProxy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 4. Agregar finalizer
	if !controllerutil.ContainsFinalizer(&mcProxy, finalizerName) {
		controllerutil.AddFinalizer(&mcProxy, finalizerName)
		if err := r.Update(ctx, &mcProxy); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 5. Validar edición
	edition := string(mcProxy.Spec.Edition)
	handler, ok := r.HandlerRegistry.Get(edition)
	if !ok {
		logger.Error(nil, "edición no soportada", "edition", edition)
		meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
			Type:    "RouteConfigured",
			Status:  metav1.ConditionFalse,
			Reason:  "UnsupportedEdition",
			Message: "La edición '" + edition + "' no está soportada",
		})
		r.Status().Update(ctx, &mcProxy)
		return ctrl.Result{}, nil
	}

	// 6. Namespace del backend
	backendNamespace := mcProxy.Spec.Backend.Namespace
	if backendNamespace == "" {
		backendNamespace = mcProxy.Namespace
	}

	// 7. Service port default
	servicePort := mcProxy.Spec.Backend.ServicePort
	if servicePort == 0 {
		servicePort = minecraftv1alpha1.DefaultServicePort(mcProxy.Spec.Edition)
	}

	// 8. Asignar puerto para Bedrock
	assignedPort := mcProxy.Spec.AssignedPort
	if mcProxy.Spec.Edition == minecraftv1alpha1.EditionBedrock && assignedPort == 0 {
		var err error
		assignedPort, err = r.PortManager.Allocate(mcProxy.Spec.Hostname)
		if err != nil {
			logger.Error(err, "error asignando puerto Bedrock")
			meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
				Type:    "RouteConfigured",
				Status:  metav1.ConditionFalse,
				Reason:  "PortAllocationFailed",
				Message: err.Error(),
			})
			r.Status().Update(ctx, &mcProxy)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// Guardar en spec para persistencia
		mcProxy.Spec.AssignedPort = assignedPort
		if err := r.Update(ctx, &mcProxy); err != nil {
			r.PortManager.Release(assignedPort)
			return ctrl.Result{}, err
		}
	} else if mcProxy.Spec.Edition == minecraftv1alpha1.EditionBedrock && assignedPort > 0 {
		// Puerto especificado manualmente, intentar reservar
		if err := r.PortManager.AllocateSpecific(assignedPort, mcProxy.Spec.Hostname); err != nil {
			// Si ya está reservado por este hostname, está bien
			logger.V(1).Info("puerto ya reservado o no disponible", "port", assignedPort, "error", err)
		}
	}

	// 9. Crear Backend
	backend := &proxy.Backend{
		ServiceName:  mcProxy.Spec.Backend.ServiceName,
		ServicePort:  servicePort,
		Namespace:    backendNamespace,
		MaxPlayers:   mcProxy.Spec.MaxPlayers,
		Edition:      edition,
		AssignedPort: assignedPort,
		Hostname:     mcProxy.Spec.Hostname,
	}

	// 10. Registrar en RouteTable según edición
	switch mcProxy.Spec.Edition {
	case minecraftv1alpha1.EditionJava:
		r.RouteTable.SetHostnameRoute(mcProxy.Spec.Hostname, backend)
	case minecraftv1alpha1.EditionBedrock:
		r.RouteTable.SetPortRoute(assignedPort, backend)
	}

	// 11. Notificar al handler
	if err := handler.AddRoute(mcProxy.Spec.Hostname, backend, assignedPort); err != nil {
		logger.Error(err, "error notificando al handler")
	}

	logger.Info("ruta configurada",
		"edition", edition,
		"hostname", mcProxy.Spec.Hostname,
		"backend", backend.Address(),
		"assignedPort", assignedPort,
	)

	// 12. Actualizar status
	mcProxy.Status.Ready = true
	mcProxy.Status.Edition = mcProxy.Spec.Edition
	mcProxy.Status.AssignedPort = assignedPort
	mcProxy.Status.ActiveConnections = backend.ActiveConnections.Load()

	meta.SetStatusCondition(&mcProxy.Status.Conditions, metav1.Condition{
		Type:               "RouteConfigured",
		Status:             metav1.ConditionTrue,
		Reason:             "RouteActive",
		Message:            "Ruta activa vía " + edition + " handler",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &mcProxy); err != nil {
		logger.Error(err, "error actualizando status")
		return ctrl.Result{}, err
	}

	// 13. Re-queue periódico
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager registra el controller.
func (r *MinecraftProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&minecraftv1alpha1.MinecraftProxy{}).
		Complete(r)
}
```

---

## Paso 9: Entrypoint (`cmd/main.go`)

### 9.1 Archivo: `cmd/main.go`

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	minecraftv1alpha1 "github.com/tuuser/mc-proxy-operator/api/v1alpha1"
	"github.com/tuuser/mc-proxy-operator/internal/controller"
	"github.com/tuuser/mc-proxy-operator/internal/proxy"
	"github.com/tuuser/mc-proxy-operator/internal/proxy/bedrock"
	"github.com/tuuser/mc-proxy-operator/internal/proxy/java"
	"github.com/tuuser/mc-proxy-operator/internal/proxy/portmanager"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(minecraftv1alpha1.AddToScheme(scheme))
}

func main() {
	opts := zap.Options{Development: true}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Configuración desde env vars
	metricsAddr := envOrDefault("METRICS_ADDR", ":8080")
	probeAddr := envOrDefault("HEALTH_PROBE_ADDR", ":8081")
	javaAddr := envOrDefault("JAVA_LISTEN_ADDR", ":25565")

	// 1. Crear componentes compartidos
	routeTable := proxy.NewRouteTable()
	portMgr := portmanager.NewPortManager(
		portmanager.DefaultMinPort,
		portmanager.DefaultMaxPort,
	)

	// 2. Crear y registrar protocol handlers (Strategy Pattern)
	registry := proxy.NewHandlerRegistry()

	javaHandler := java.NewJavaProtocolHandler(javaAddr)
	registry.Register(javaHandler)

	bedrockHandler := bedrock.NewBedrockProtocolHandler()
	registry.Register(bedrockHandler)

	// Para agregar una nueva edición en el futuro:
	// geyserHandler := geyser.NewGeyserProtocolHandler(...)
	// registry.Register(geyserHandler)

	setupLog.Info("protocol handlers registrados",
		"editions", registry.ListEditions(),
	)

	// 3. Crear el manager de controller-runtime
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "error creando manager")
		os.Exit(1)
	}

	// 4. Registrar el reconciler con TODAS las dependencias
	if err := (&controller.MinecraftProxyReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		RouteTable:      routeTable,
		HandlerRegistry: registry,
		PortManager:     portMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "error configurando controller")
		os.Exit(1)
	}

	// 5. Health checks
	mgr.AddHealthzCheck("healthz", healthz.Ping)
	mgr.AddReadyzCheck("readyz", healthz.Ping)

	// 6. Contexto con cancelación por señales
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 7. Iniciar TODOS los handlers en goroutines
	if err := registry.StartAll(ctx, routeTable); err != nil {
		setupLog.Error(err, "error iniciando handlers")
		os.Exit(1)
	}

	// 8. Iniciar el manager (bloquea)
	setupLog.Info("iniciando manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "error ejecutando manager")
		os.Exit(1)
	}

	// 9. Cleanup
	registry.StopAll()
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
```

---

## Paso 10: Dockerfile

```dockerfile
# Archivo: Dockerfile

# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o manager cmd/main.go

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .

# Java Edition TCP
EXPOSE 25565
# Bedrock Edition UDP range
EXPOSE 19132-19232/udp
# Metrics
EXPOSE 8080
# Health probes
EXPOSE 8081

USER 65532:65532

ENTRYPOINT ["/manager"]
```

---

## Paso 11: Manifiestos de Despliegue

### 11.1 Namespace: `deploy/namespace.yaml`

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: minecraft-system
  labels:
    app.kubernetes.io/name: mc-proxy-operator
---
apiVersion: v1
kind: Namespace
metadata:
  name: mc-servers
  labels:
    app.kubernetes.io/name: mc-proxy-operator
    app.kubernetes.io/component: game-servers
```

### 11.2 Service Java: `deploy/service-java.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mc-proxy-java
  namespace: minecraft-system
spec:
  type: LoadBalancer
  ports:
    - port: 25565
      targetPort: 25565
      protocol: TCP
      name: minecraft-java
  selector:
    control-plane: controller-manager
```

### 11.3 Service Bedrock: `deploy/service-bedrock.yaml`

```yaml
# Para Bedrock necesitamos exponer un rango de puertos UDP.
# Opción A: Múltiples puertos en un Service (limitado).
# Opción B (recomendada): hostNetwork o NodePort por cada servidor.
#
# La opción más práctica para producción es usar hostNetwork: true
# en el Deployment del operator, lo que expone directamente los puertos UDP.
# Alternativamente, se puede usar un Service por cada servidor Bedrock
# creado dinámicamente por el operator (mejora futura).

# Service base para los primeros 100 puertos Bedrock
apiVersion: v1
kind: Service
metadata:
  name: mc-proxy-bedrock
  namespace: minecraft-system
spec:
  type: LoadBalancer
  selector:
    control-plane: controller-manager
  ports:
    - port: 19132
      targetPort: 19132
      protocol: UDP
      name: bedrock-default
    # Nota: en producción, el operator debería crear Services dinámicos
    # o usar hostNetwork para evitar tener que listar cada puerto aquí.
```

### 11.4 Deployment: `config/manager/manager.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mc-proxy-operator
  namespace: minecraft-system
  labels:
    control-plane: controller-manager
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      serviceAccountName: mc-proxy-operator
      terminationGracePeriodSeconds: 60
      # hostNetwork permite que los puertos UDP de Bedrock sean accesibles
      # directamente sin crear Services por cada puerto.
      # En producción, evaluar si se prefiere crear Services dinámicos.
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      containers:
        - name: manager
          image: ghcr.io/tuuser/mc-proxy-operator:latest
          ports:
            - containerPort: 25565
              name: mc-java
              protocol: TCP
            - containerPort: 8080
              name: metrics
              protocol: TCP
            - containerPort: 8081
              name: health
              protocol: TCP
          env:
            - name: JAVA_LISTEN_ADDR
              value: ":25565"
            - name: METRICS_ADDR
              value: ":8080"
            - name: HEALTH_PROBE_ADDR
              value: ":8081"
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8081
            initialDelaySeconds: 15
            periodSeconds: 20
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8081
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 200m
              memory: 256Mi
            limits:
              cpu: "1"
              memory: 512Mi
```

### 11.5 RBAC

```yaml
# config/rbac/service_account.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mc-proxy-operator
  namespace: minecraft-system
---
# config/rbac/role.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mc-proxy-operator-role
rules:
  - apiGroups: ["minecraft.miminecraftserver.com"]
    resources: ["minecraftproxies"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["minecraft.miminecraftserver.com"]
    resources: ["minecraftproxies/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["minecraft.miminecraftserver.com"]
    resources: ["minecraftproxies/finalizers"]
    verbs: ["update"]
---
# config/rbac/role_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mc-proxy-operator-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: mc-proxy-operator-role
subjects:
  - kind: ServiceAccount
    name: mc-proxy-operator
    namespace: minecraft-system
```

---

## Paso 12: Ejemplos de Uso Completo

### 12.1 Servidor Java: `deploy/examples/java-server-jugador1.yaml`

```yaml
# Deployment del servidor Minecraft Java
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jugador1-minecraft
  namespace: mc-servers
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minecraft
      user: jugador1
      edition: java
  template:
    metadata:
      labels:
        app: minecraft
        user: jugador1
        edition: java
    spec:
      containers:
        - name: minecraft
          image: itzg/minecraft-server:latest
          ports:
            - containerPort: 25565
          env:
            - name: EULA
              value: "TRUE"
            - name: TYPE
              value: "PAPER"
            - name: MEMORY
              value: "2G"
          resources:
            requests:
              cpu: 500m
              memory: 2Gi
            limits:
              cpu: "2"
              memory: 4Gi
---
apiVersion: v1
kind: Service
metadata:
  name: jugador1-minecraft
  namespace: mc-servers
spec:
  selector:
    app: minecraft
    user: jugador1
  ports:
    - port: 25565
      targetPort: 25565
      protocol: TCP
---
# MinecraftProxy CR para Java
apiVersion: minecraft.miminecraftserver.com/v1alpha1
kind: MinecraftProxy
metadata:
  name: jugador1-java
  namespace: minecraft-system
spec:
  edition: java
  hostname: "jugador1.miminecraftserver.com"
  backend:
    serviceName: "jugador1-minecraft"
    servicePort: 25565
    namespace: "mc-servers"
  maxPlayers: 20
```

### 12.2 Servidor Bedrock: `deploy/examples/bedrock-server-jugador2.yaml`

```yaml
# Deployment del servidor Minecraft Bedrock
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jugador2-minecraft-bedrock
  namespace: mc-servers
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minecraft
      user: jugador2
      edition: bedrock
  template:
    metadata:
      labels:
        app: minecraft
        user: jugador2
        edition: bedrock
    spec:
      containers:
        - name: minecraft-bedrock
          image: itzg/minecraft-bedrock-server:latest
          ports:
            - containerPort: 19132
              protocol: UDP
          env:
            - name: EULA
              value: "TRUE"
            - name: GAMEMODE
              value: "survival"
          resources:
            requests:
              cpu: 500m
              memory: 1Gi
            limits:
              cpu: "2"
              memory: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: jugador2-minecraft-bedrock
  namespace: mc-servers
spec:
  selector:
    app: minecraft
    user: jugador2
  ports:
    - port: 19132
      targetPort: 19132
      protocol: UDP
---
# MinecraftProxy CR para Bedrock — puerto se auto-asigna
apiVersion: minecraft.miminecraftserver.com/v1alpha1
kind: MinecraftProxy
metadata:
  name: jugador2-bedrock
  namespace: minecraft-system
spec:
  edition: bedrock
  hostname: "jugador2.miminecraftserver.com"
  backend:
    serviceName: "jugador2-minecraft-bedrock"
    servicePort: 19132
    namespace: "mc-servers"
  maxPlayers: 10
  # assignedPort se omite: el operator asigna automáticamente
```

---

## Paso 13: Makefile

```makefile
IMG ?= ghcr.io/tuuser/mc-proxy-operator:latest

.PHONY: all
all: generate manifests build test

## Generación
.PHONY: generate
generate:
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests:
	controller-gen rbac:roleName=mc-proxy-operator-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

## Build
.PHONY: build
build: generate
	go build -o bin/manager cmd/main.go

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

## Testing
.PHONY: test
test: generate
	go test ./... -v -race -coverprofile=coverage.out

.PHONY: test-coverage
test-coverage: test
	go tool cover -html=coverage.out -o coverage.html

## Despliegue
.PHONY: install
install: manifests
	kubectl apply -f config/crd/bases/

.PHONY: deploy
deploy: manifests
	kubectl apply -f deploy/namespace.yaml
	kubectl apply -f config/crd/bases/
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/manager.yaml
	kubectl apply -f deploy/service-java.yaml
	kubectl apply -f deploy/service-bedrock.yaml

.PHONY: undeploy
undeploy:
	kubectl delete -f deploy/ --ignore-not-found
	kubectl delete -f config/manager/ --ignore-not-found
	kubectl delete -f config/rbac/ --ignore-not-found
	kubectl delete -f config/crd/bases/ --ignore-not-found

## Desarrollo local
.PHONY: run
run: generate
	go run cmd/main.go

.PHONY: clean
clean:
	rm -rf bin/ coverage.out coverage.html

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
```

---

## Paso 14: Guía para agregar una nueva edición (extensibilidad)

El Strategy Pattern hace que agregar soporte para una nueva edición sea un proceso bien definido:

### 14.1 Crear el paquete del handler

```bash
mkdir -p internal/proxy/nuevaedicion/
```

### 14.2 Implementar la interfaz `ProtocolHandler`

```go
// internal/proxy/nuevaedicion/handler.go
package nuevaedicion

import (
	"context"
	"github.com/tuuser/mc-proxy-operator/internal/proxy"
)

type NuevaEdicionHandler struct {
	// campos específicos
}

func (h *NuevaEdicionHandler) Name() string            { return "nuevaedicion" }
func (h *NuevaEdicionHandler) DefaultPort() int32       { return 12345 }
func (h *NuevaEdicionHandler) Start(ctx context.Context, rt *proxy.RouteTable) error { /* ... */ }
func (h *NuevaEdicionHandler) Stop() error              { /* ... */ }
func (h *NuevaEdicionHandler) AddRoute(hostname string, backend *proxy.Backend, port int32) error { /* ... */ }
func (h *NuevaEdicionHandler) RemoveRoute(hostname string, port int32) error { /* ... */ }
```

### 14.3 Actualizar el enum de Edition en el CRD

En `api/v1alpha1/minecraftproxy_types.go`:

```go
// +kubebuilder:validation:Enum=java;bedrock;nuevaedicion
type Edition string

const (
	EditionJava         Edition = "java"
	EditionBedrock      Edition = "bedrock"
	EditionNuevaEdicion Edition = "nuevaedicion"
)
```

### 14.4 Registrar en `cmd/main.go`

```go
nuevaHandler := nuevaedicion.NewNuevaEdicionHandler(...)
registry.Register(nuevaHandler)
```

### 14.5 Agregar lógica de routing al reconciler (si usa un método diferente)

En el reconciler, agregar el case para la nueva edición en el switch de routing.

### 14.6 Ejecutar generación

```bash
make generate
make manifests
```

### Checklist para nueva edición

- [ ] Crear paquete en `internal/proxy/nuevaedicion/`
- [ ] Implementar `ProtocolHandler` interface completa
- [ ] Escribir parser del protocolo específico
- [ ] Escribir tests unitarios del parser
- [ ] Escribir tests de integración del handler
- [ ] Agregar valor al enum `Edition` en types
- [ ] Agregar `DefaultServicePort` case
- [ ] Registrar handler en `main.go`
- [ ] Agregar case en el reconciler si el routing es diferente
- [ ] Agregar case en la lógica de eliminación del reconciler
- [ ] Crear ejemplo de CR en `config/samples/`
- [ ] Crear ejemplo completo en `deploy/examples/`
- [ ] Actualizar README con documentación de la nueva edición
- [ ] `make generate && make manifests && make test`

---

## Paso 15: Consideraciones Técnicas y Edge Cases

### 15.1 Java Edition: Forge/FML

Clientes Forge agregan sufijos al hostname: `\x00FML\x00`, `\x00FML2\x00`, `\x00FML3\x00`. La función `cleanServerAddress()` maneja esto.

### 15.2 Java Edition: SRV Records

Algunos clientes resuelven `_minecraft._tcp.hostname` vía DNS SRV. El proxy no se ve afectado porque el handshake siempre contiene el hostname original.

### 15.3 Bedrock Edition: hostNetwork vs Services dinámicos

Para Bedrock, cada servidor necesita un puerto UDP. Hay dos approaches:

- **hostNetwork: true** (implementado en esta versión): el pod del operator usa la red del host directamente, exponiendo todos los puertos sin Services intermedios. Más simple pero limita a 1 réplica por nodo.
- **Services dinámicos** (mejora futura): el operator crea un Service NodePort/LoadBalancer por cada servidor Bedrock. Más complejo pero más flexible.

### 15.4 Graceful Shutdown

El proxy maneja SIGTERM: cierra listeners, espera conexiones activas (hasta `terminationGracePeriodSeconds`), y luego termina.

### 15.5 Hostname duplicados

Dos CRs no deben tener el mismo hostname (Java) o el mismo puerto (Bedrock). En esta versión, la route table simplemente sobrescribe. Mejora futura: implementar un Validating Webhook.

### 15.6 Persistencia de puertos Bedrock

Cuando el operator se reinicia, el `PortManager` empieza vacío. El reconciler re-procesa todos los CRs existentes y re-registra los puertos ya asignados (almacenados en `spec.assignedPort`). Esto garantiza que los puertos se preservan entre reinicios.

### 15.7 Proxy Protocol (futuro)

Para preservar la IP real del jugador cuando hay un L4 LB delante, implementar soporte para PROXY Protocol v2. Esto aplica tanto para Java (TCP) como potencialmente para Bedrock (con headers customizados).

---

## Resumen de Archivos a Implementar

| # | Archivo | Descripción | Prioridad |
|---|---|---|---|
| 1 | `api/v1alpha1/minecraftproxy_types.go` | CRD con campo Edition | P0 |
| 2 | `api/v1alpha1/groupversion_info.go` | Registro del grupo | P0 |
| 3 | `internal/proxy/handler.go` | Interface ProtocolHandler + Registry | P0 |
| 4 | `internal/proxy/handler_test.go` | Tests del registry | P0 |
| 5 | `internal/proxy/routetable.go` | Route table dual (hostname + port) | P0 |
| 6 | `internal/proxy/routetable_test.go` | Tests route table | P0 |
| 7 | `internal/proxy/portmanager/manager.go` | Port manager para Bedrock | P0 |
| 8 | `internal/proxy/portmanager/manager_test.go` | Tests port manager | P0 |
| 9 | `internal/proxy/java/minecraft.go` | Parser handshake Java | P0 |
| 10 | `internal/proxy/java/minecraft_test.go` | Tests parser Java | P0 |
| 11 | `internal/proxy/java/handler.go` | TCP proxy handler Java | P0 |
| 12 | `internal/proxy/bedrock/raknet.go` | Parser básico RakNet | P0 |
| 13 | `internal/proxy/bedrock/raknet_test.go` | Tests RakNet | P0 |
| 14 | `internal/proxy/bedrock/handler.go` | UDP proxy handler Bedrock | P0 |
| 15 | `internal/proxy/bedrock/handler_test.go` | Tests handler Bedrock | P1 |
| 16 | `internal/controller/minecraftproxy_controller.go` | Reconciler multi-edition | P0 |
| 17 | `cmd/main.go` | Entrypoint con registry | P0 |
| 18 | `Dockerfile` | Multi-stage build | P0 |
| 19 | `Makefile` | Automatización | P0 |
| 20 | `deploy/namespace.yaml` | Namespaces | P1 |
| 21 | `deploy/service-java.yaml` | Service TCP | P1 |
| 22 | `deploy/service-bedrock.yaml` | Service UDP | P1 |
| 23 | `config/manager/manager.yaml` | Deployment | P1 |
| 24 | `config/rbac/*.yaml` | RBAC | P1 |
| 25 | `config/samples/*.yaml` | Ejemplos de CR | P1 |
| 26 | `deploy/examples/*.yaml` | Ejemplos completos | P1 |

### Orden de implementación recomendado

1. Scaffolding con Kubebuilder (`kubebuilder init` + `create api`)
2. Definir types del CRD con Edition enum (archivo #1, #2)
3. `make generate && make manifests`
4. Implementar ProtocolHandler interface + Registry (archivos #3, #4)
5. Implementar RouteTable dual (archivos #5, #6)
6. Implementar PortManager (archivos #7, #8)
7. Implementar parser Java + tests (archivos #9, #10)
8. Implementar Java handler TCP proxy (archivo #11)
9. Implementar parser RakNet básico + tests (archivos #12, #13)
10. Implementar Bedrock handler UDP proxy (archivos #14, #15)
11. Implementar Reconciler multi-edition (archivo #16)
12. Implementar main.go con registry (archivo #17)
13. Dockerfile y Makefile (archivos #18, #19)
14. Manifiestos de despliegue (archivos #20-26)
15. Build, test, deploy completo

---

## Comandos de Verificación

```bash
# Compilar
make build

# Tests con race detector
make test

# Generar CRDs
make manifests

# Docker build
make docker-build

# Desplegar
make deploy

# Crear servidor Java
kubectl apply -f deploy/examples/java-server-jugador1.yaml

# Crear servidor Bedrock
kubectl apply -f deploy/examples/bedrock-server-jugador2.yaml

# Ver todos los proxies
kubectl get minecraftproxies -A
# NAME                EDITION   HOSTNAME                            BACKEND                        PORT    READY   CONNECTIONS   AGE
# jugador1-java       java      jugador1.miminecraftserver.com      jugador1-minecraft             0       true    3             5m
# jugador2-bedrock    bedrock   jugador2.miminecraftserver.com      jugador2-minecraft-bedrock     19132   true    1             3m

# Ver logs
kubectl logs -n minecraft-system deployment/mc-proxy-operator -f

# Probar Java (desde Minecraft → Direct Connect)
# jugador1.miminecraftserver.com:25565

# Probar Bedrock (desde Minecraft → Add Server)
# IP: miminecraftserver.com  Puerto: 19132 (el asignado por el operator)
```
