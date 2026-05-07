# MC Proxy Operator

Kubernetes Operator escrito en Go que actúa como **reverse proxy multi-protocolo para servidores de Minecraft**. Soporta Java Edition (TCP) y Bedrock Edition (UDP/RakNet), permitiendo que múltiples usuarios tengan sus propios servidores dentro de un clúster de Kubernetes, accesibles por subdominio.

## Tabla de contenidos

- [Arquitectura](#arquitectura)
- [Prerrequisitos](#prerrequisitos)
- [Desarrollo local](#desarrollo-local)
- [Deployment en Kubernetes](#deployment-en-kubernetes)
- [Agregar un nuevo servidor de Minecraft](#agregar-un-nuevo-servidor-de-minecraft)
- [Continuar el desarrollo](#continuar-el-desarrollo)
- [Referencia de comandos](#referencia-de-comandos)

---

## Arquitectura

```
Jugador Java  ──TCP──►  subdominio.miminecraftserver.com:25565 ──►  mc-proxy-operator  ──►  Service (mc-servers)
Jugador Bedrock ─UDP─►  subdominio.miminecraftserver.com:19132+ ──► mc-proxy-operator  ──►  Service (mc-servers)
```

El operador gestiona el CRD `MinecraftProxy`. Cuando se crea un recurso de este tipo, el controlador registra la ruta en una tabla en memoria que los protocol handlers consultan para enrutar el tráfico:

- **JavaHandler**: escucha en TCP `:25565`, extrae el hostname del handshake packet (ID `0x00`) y enruta al backend correspondiente.
- **BedrockHandler**: cada servidor Bedrock recibe un puerto UDP dedicado asignado dinámicamente por el `PortManager` en el rango `19132–29132`.

**Namespaces:**
| Namespace | Propósito |
|---|---|
| `minecraft-system` | El operator y sus recursos RBAC |
| `mc-servers` | Los servidores de Minecraft de cada usuario |

---

## Prerrequisitos

| Herramienta | Versión mínima | Uso |
|---|---|---|
| Go | 1.25 | Compilar y ejecutar localmente |
| Docker | 20+ | Construir imagen del operador |
| kubectl | 1.28+ | Interactuar con el clúster |
| Acceso a un clúster K8s | — | GKE, EKS, Kind, etc. |

Para desarrollo también se necesita:

| Herramienta | Propósito |
|---|---|
| `make` | Automatización de tareas |
| `kind` | Clúster local para e2e tests |

---

## Desarrollo local

### 1. Clonar el repositorio

```bash
git clone https://github.com/luisito666/mc-proxy-operator.git
cd mc-proxy-operator
```

### 2. Instalar dependencias de Go

```bash
go mod download
```

### 3. Generar CRDs y código

Cada vez que se modifiquen los tipos en `api/v1alpha1/`, se deben regenerar los manifests y el código de deepcopy:

```bash
make manifests generate
```

### 4. Instalar las CRDs en el clúster activo

```bash
make install
```

Esto aplica los CRDs al clúster configurado en `~/.kube/config`.

### 5. Ejecutar el operador localmente (fuera del clúster)

```bash
make run
```

El operador corre en tu máquina pero se conecta al clúster de Kubernetes. Variables de entorno opcionales:

| Variable | Default | Descripción |
|---|---|---|
| `JAVA_LISTEN_ADDR` | `:25565` | Puerto TCP para Java Edition |
| `METRICS_ADDR` | `:8080` | Puerto para métricas Prometheus |
| `HEALTH_PROBE_ADDR` | `:8081` | Puerto para health checks |

### 6. Correr tests

```bash
# Tests unitarios e integración
make test

# Tests e2e (requiere kind instalado)
make test-e2e
```

---

## Deployment en Kubernetes

### Paso 1 — Construir y publicar la imagen Docker

Reemplaza `<REGISTRY>` con tu registry (ej: `ghcr.io/luisito666`).

```bash
# Construir la imagen
make docker-build IMG=<REGISTRY>/mc-proxy-operator:v1.0.0

# Publicar al registry
make docker-push IMG=<REGISTRY>/mc-proxy-operator:v1.0.0
```

Para arquitecturas múltiples (AMD64 + ARM64):

```bash
make docker-buildx IMG=<REGISTRY>/mc-proxy-operator:v1.0.0
```

### Paso 2 — Crear los namespaces

```bash
kubectl apply -f deploy/namespace.yaml
```

Esto crea `minecraft-system` (para el operador) y `mc-servers` (para los servidores de juego).

### Paso 3 — Aplicar RBAC

```bash
kubectl apply -f deploy/service_account.yaml
kubectl apply -f deploy/role.yaml
kubectl apply -f deploy/role_binding.yaml
```

### Paso 4 — Instalar el CRD `MinecraftProxy`

```bash
make install
```

Verifica que el CRD quedó registrado:

```bash
kubectl get crd minecraftproxies.minecraft.miminecraftserver.com
```

### Paso 5 — Desplegar el operador

Edita `deploy/manager.yaml` y actualiza el campo `image` con la imagen que publicaste en el Paso 1:

```yaml
containers:
  - name: manager
    image: <REGISTRY>/mc-proxy-operator:v1.0.0   # <-- actualizar aquí
```

Luego aplica:

```bash
kubectl apply -f deploy/manager.yaml
```

Verifica que el pod esté corriendo:

```bash
kubectl get pods -n minecraft-system
# NAME                              READY   STATUS    RESTARTS   AGE
# mc-proxy-operator-xxxxx           1/1     Running   0          30s
```

### Paso 6 — Exponer el operador con LoadBalancer

```bash
# Servicio para Java Edition (TCP 25565)
kubectl apply -f deploy/service-java.yaml

# Servicio para Bedrock Edition (UDP 19132)
kubectl apply -f deploy/service-bedrock.yaml
```

Obtén la IP externa del LoadBalancer:

```bash
kubectl get svc -n minecraft-system
# NAME               TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)
# mc-proxy-java      LoadBalancer   10.x.x.x       <IP-PUBLICA>    25565/TCP
# mc-proxy-bedrock   LoadBalancer   10.x.x.x       <IP-PUBLICA>    19132/UDP
```

### Paso 7 — Configurar DNS

Crea registros DNS que apunten a la IP pública obtenida en el paso anterior:

```
*.miminecraftserver.com  →  <IP-PUBLICA>
```

Con esto, cualquier subdominio (`jugador1.miminecraftserver.com`, `jugador2.miminecraftserver.com`, etc.) resolverá al proxy.

### Paso 8 — Verificar el deployment completo

```bash
# El operador está listo
kubectl get pods -n minecraft-system

# El CRD está instalado
kubectl get crd minecraftproxies.minecraft.miminecraftserver.com

# Los servicios tienen IP externa
kubectl get svc -n minecraft-system

# No hay errores en logs del operador
kubectl logs -n minecraft-system deployment/mc-proxy-operator
```

---

## Agregar un nuevo servidor de Minecraft

Una vez el operador está corriendo, agregar un servidor nuevo es aplicar dos manifests: el servidor de Minecraft y el `MinecraftProxy` que registra la ruta.

### Servidor Java Edition

```bash
kubectl apply -f deploy/examples/java-server-jugador1.yaml
```

Este archivo crea el `Deployment` del servidor, su `Service`, y el `MinecraftProxy`:

```yaml
apiVersion: minecraft.miminecraftserver.com/v1alpha1
kind: MinecraftProxy
metadata:
  name: jugador1-java
  namespace: minecraft-system
spec:
  edition: java
  hostname: "jugador1.miminecraftserver.com"  # subdominio del jugador
  backend:
    serviceName: "jugador1-minecraft"          # nombre del Service en mc-servers
    servicePort: 25565
    namespace: "mc-servers"
  maxPlayers: 20
```

El jugador se conecta a `jugador1.miminecraftserver.com:25565`.

### Servidor Bedrock Edition

```bash
kubectl apply -f deploy/examples/bedrock-server-jugador2.yaml
```

Para Bedrock, el operador asigna automáticamente un puerto UDP en el rango `19132–29132`. Consulta el puerto asignado con:

```bash
kubectl get minecraftproxy jugador2-bedrock -n minecraft-system
# NAME               EDITION   HOSTNAME                              PORT    READY
# jugador2-bedrock   bedrock   jugador2.miminecraftserver.com        19133   true
```

El jugador se conecta a `jugador2.miminecraftserver.com:<PORT>`.

### Verificar rutas activas

```bash
kubectl get minecraftproxies -n minecraft-system
```

---

## Continuar el desarrollo

### Estructura del proyecto

```
.
├── api/v1alpha1/           # Tipos del CRD (MinecraftProxy, spec, status)
├── cmd/main.go             # Entrypoint: inicializa manager, handlers, controller
├── internal/
│   ├── controller/         # Reconciler de MinecraftProxy
│   └── proxy/
│       ├── handler.go      # Interface ProtocolHandler + HandlerRegistry
│       ├── routetable.go   # Tabla hostname → backend en memoria
│       ├── java/           # Handler TCP para Java Edition
│       ├── bedrock/        # Handler UDP para Bedrock Edition
│       └── portmanager/    # Asignación dinámica de puertos UDP
├── deploy/                 # Manifests de Kubernetes listos para usar
│   └── examples/           # Ejemplos de servidores Java y Bedrock
├── config/                 # Manifests generados por kubebuilder (RBAC, CRD, etc.)
└── test/                   # Tests unitarios y e2e
```

### Agregar soporte para una nueva edición de Minecraft

1. Crear el directorio `internal/proxy/<nueva-edicion>/`
2. Implementar la interfaz `ProtocolHandler` definida en `internal/proxy/handler.go`:

```go
type ProtocolHandler interface {
    Name() string
    DefaultPort() int32
    Start(ctx context.Context, rt RouteTable) error
    Stop() error
}
```

3. Agregar el nuevo valor al enum `Edition` en `api/v1alpha1/minecraftproxy_types.go`
4. Registrar el handler en `cmd/main.go`:

```go
myHandler := mynewedition.NewHandler()
registry.Register(myHandler)
```

5. Regenerar CRDs y código:

```bash
make manifests generate
```

### Flujo de trabajo típico

```bash
# 1. Hacer cambios en el código
# 2. Regenerar si se modificaron tipos en api/
make manifests generate

# 3. Formatear y verificar
make fmt vet

# 4. Correr tests
make test

# 5. Construir y publicar nueva imagen
make docker-build docker-push IMG=<REGISTRY>/mc-proxy-operator:<nueva-version>

# 6. Actualizar el deployment en el clúster
kubectl set image deployment/mc-proxy-operator \
  manager=<REGISTRY>/mc-proxy-operator:<nueva-version> \
  -n minecraft-system

# 7. Verificar rollout
kubectl rollout status deployment/mc-proxy-operator -n minecraft-system
```

### Lint

```bash
# Verificar linting
make lint

# Aplicar fixes automáticos
make lint-fix
```

---

## Referencia de comandos

| Comando | Descripción |
|---|---|
| `make build` | Compilar el binario en `bin/manager` |
| `make run` | Ejecutar el operador localmente contra el clúster |
| `make test` | Correr tests unitarios y de integración |
| `make test-e2e` | Correr tests e2e con Kind |
| `make docker-build IMG=...` | Construir imagen Docker |
| `make docker-push IMG=...` | Publicar imagen al registry |
| `make install` | Instalar CRDs en el clúster |
| `make uninstall` | Desinstalar CRDs del clúster |
| `make deploy IMG=...` | Desplegar el operador via kustomize |
| `make undeploy` | Eliminar el operador del clúster |
| `make manifests` | Regenerar CRDs y RBAC desde los tipos |
| `make generate` | Regenerar código DeepCopy |
| `make lint` | Correr golangci-lint |
