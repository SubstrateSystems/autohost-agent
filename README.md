# Autohost Agent

Agente de monitorización y ejecución remota para Linux. Se conecta a la [autohost-cloud-api](../autohost-cloud-api), envía heartbeats y métricas, y ejecuta comandos remotos via gRPC.

## Stack

- **Go** 1.25+
- **gRPC** (recepción de comandos remotos)
- **WebSocket** (Gorilla WS)
- **systemd** (modo servicio)

## Requisitos previos

- Linux (amd64 o arm64)
- Go 1.25+ (solo para compilar desde fuente)
- La API ([autohost-cloud-api](../autohost-cloud-api)) accesible desde el nodo
- Un token de enrollment generado desde el dashboard

---

## Instalación

### Opción 1 — Script automático (recomendado)

Descarga el binario, crea el usuario de sistema `autohost`, genera `/etc/autohost/config.yaml` y registra el servicio systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh | bash
```

El script pedirá de forma interactiva:
- URL de la API (`AUTOHOST_API_URL`)
- Token de enrollment (`AUTOHOST_TOKEN`)
- ID del nodo (default: `hostname`)
- Tags opcionales

### Instalación no-interactiva (CI / automatización)

```bash
AUTOHOST_API_URL=https://mi-api.example.com \
AUTOHOST_TOKEN=autohost-node_xxxx \
AUTOHOST_NODE_ID=mi-servidor \
AUTOHOST_TAGS="production,web" \
bash <(curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh)
```

### Versión específica

```bash
VERSION=v0.2.0 bash <(curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh)
```

### Verificar instalación

```bash
autohost-agent --version
systemctl status autohost-agent
```

---

### Opción 2 — Compilar desde fuente

```bash
git clone https://github.com/mazapanuwu13/autohost-agent.git
cd autohost-agent
make build
```

El binario queda en `./autohost-agent`. Para instalar como servicio:

```bash
make install   # copia binario + servicio systemd + config inicial
make enable    # habilita e inicia el servicio
```

---

## Configuración

El archivo de configuración vive en `/etc/autohost/config.yaml` (permisos `600`):

```yaml
api_url: "http://192.168.1.10:8080"   # URL HTTP del backend
ws_url: ""                             # reservado
grpc_address: "192.168.1.10:9090"     # dirección gRPC del backend
agent_token: "autohost-node_xxxx"     # token de nodo obtenido en el enrollment
node_id: "mi-servidor"                # ID único del nodo
tags: ["production", "web"]
```

> Este archivo es generado automáticamente por `autohost up` (autohost-cli) o por el script de instalación. No editarlo manualmente salvo casos excepcionales.

---

## Comandos Make

```bash
make build       # Compilar binario en ./autohost-agent
make run         # Compilar y ejecutar con config.example.yaml
make test        # Ejecutar tests
make clean       # Eliminar binario compilado
make install     # Instalar binario + servicio systemd (requiere sudo)
make uninstall   # Desinstalar (preserva /etc/autohost/)
make enable      # Habilitar e iniciar el servicio systemd
make disable     # Detener y deshabilitar el servicio
make status      # Ver estado del servicio
make logs        # Seguir logs en tiempo real (journalctl -f)
make release     # Compilar binarios multi-plataforma + tag git en dist/
```

---

## Estructura del proyecto

```
.
├── cmd/agent/            # Entry point (acepta <config-path> o --version)
├── internal/
│   ├── agent/            # Orquestador principal (config, lifecycle, run loop)
│   ├── api/              # Cliente HTTP hacia la cloud API
│   ├── commands/         # Registro de comandos (built-in + scripts custom)
│   ├── domain/           # Tipos de dominio (Job, App, CustomCommand…)
│   ├── grpc/             # Cliente gRPC (recepción de trabajos remotos)
│   ├── services/         # Heartbeat, métricas, enrollment
│   ├── security/         # Identidad del nodo y firma de requests
│   ├── transport/        # gRPC client + WebSocket client
│   └── adapters/         # Adaptadores de infraestructura
├── pkg/sysinfo/          # Lectura de CPU, memoria y disco
├── configs/agent.yaml    # Plantilla de configuración
├── scripts/install.sh    # Instalador automático
├── autohost-agent.service # Unidad systemd
└── Makefile
```

---

## Funcionalidades

### Heartbeat
- **Endpoint:** `POST /v1/heartbeats/heartbeat`
- **Frecuencia:** cada 15 s
- Envía: `node_id`, `hostname`, `os`, `uptime_seconds`, `tags`

### Métricas del sistema
- **Endpoint:** `POST /v1/node-metrics/metrics`
- **Frecuencia:** cada 15 s
- Envía: uso de CPU, memoria (total/usado/disponible) y disco raíz

### Ejecución remota de comandos (gRPC)
- Se conecta al servidor gRPC del backend
- Registra los comandos disponibles en el nodo (built-in + scripts custom)
- Ejecuta trabajos recibidos y reporta el resultado (output / error / status)
- Scripts custom: archivos `.sh` en `~/.autohost/custom-commands/`

---

## Servicio systemd

El agente corre como usuario dedicado `autohost` con hardening de seguridad:

```
/etc/systemd/system/autohost-agent.service
```

Comandos útiles:

```bash
sudo systemctl status autohost-agent
sudo systemctl restart autohost-agent
sudo journalctl -u autohost-agent -f
```

---

## Seguridad

- Config en `/etc/autohost/config.yaml` con permisos `600` (solo root/autohost)
- El servicio corre sin privilegios de root (`NoNewPrivileges=true`)
- Filesystem de solo lectura excepto `StateDirectory=autohost`

## Licencia

MIT

## Estructura del Proyecto

```
autohost-agent/
├── cmd/
│   └── agent/              # Punto de entrada principal
│       └── main.go
│
├── internal/
│   ├── agent/              # Lógica principal del agente
│   │   ├── agent.go
│   │   ├── lifecycle.go
│   │   └── config.go
│   │
│   ├── enrollment/         # Registro de nuevos agentes
│   │   ├── service.go
│   │   └── token.go
│   │
│   ├── heartbeat/          # Envío de heartbeats
│   │   ├── service.go
│   │   └── payload.go
│   │
│   ├── metrics/            # Recolección de métricas
│   │   ├── collector.go
│   │   └── model.go
│   │
│   ├── jobs/               # Ejecución de trabajos
│   │   ├── runner.go
│   │   └── job.go
│   │
│   ├── transport/          # Comunicación con el backend
│   │   ├── httpclient.go
│   │   └── wsclient.go
│   │
│   └── security/           # Seguridad y autenticación
│       ├── signer.go
│       └── identity.go
│
├── pkg/
│   └── sysinfo/            # Información del sistema
│       ├── cpu.go
│       ├── memory.go
│       └── disk.go
│
├── configs/
│   └── agent.yaml          # Configuración de ejemplo
│
├── scripts/
│   └── install.sh          # Script de instalación
│
├── go.mod
└── README.md
```

## Configuración

## Instalación

### Instalador automático (recomendado)

Un solo comando: descarga el binario, crea el usuario de sistema, genera `/etc/autohost/config.yaml` y registra el servicio systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh | bash
```

El instalador te pedirá de forma interactiva:
- URL de la API (`AUTOHOST_API_URL`)
- Token de enrolamiento (`AUTOHOST_TOKEN`)
- ID del nodo (por defecto: `hostname`)
- Tags opcionales

### Instalación no-interactiva (CI / automatización)

```bash
AUTOHOST_API_URL=https://cloud.autohost.dev \
AUTOHOST_TOKEN=autohost-node_xxxx \
AUTOHOST_NODE_ID=mi-servidor \
AUTOHOST_TAGS="production,web" \
bash <(curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh)
```

### Versión específica

```bash
VERSION=v0.2.0 curl -fsSL https://raw.githubusercontent.com/mazapanuwu13/autohost-agent/main/scripts/install.sh | bash
```

### Verificar instalación

```bash
autohost-agent --version
systemctl status autohost-agent
```

### Compilación desde fuente

```bash
make build
# o con versión explícita:
make build VERSION=v0.2.0
```

### Ejecución manual (desarrollo)
```bash
./autohost-agent /etc/autohost/config.yaml
```

### Logs
```bash
journalctl -u autohost-agent -f
```

## Makefile

El proyecto incluye un Makefile con los siguientes comandos:

- `make build` - Compilar el binario
- `make clean` - Limpiar archivos compilados
- `make install` - Instalar el agente como servicio
- `make uninstall` - Desinstalar el agente
- `make enable` - Habilitar e iniciar el servicio
- `make disable` - Detener y deshabilitar el servicio

## Funcionalidades Actuales

### Heartbeat
- **Endpoint**: `POST /v1/heartbeats/heartbeat`
- **Frecuencia**: Cada 15 segundos
- **Datos enviados**:
  - `node_id`: ID único del nodo
  - `hostname`: Nombre del host
  - `tags`: Etiquetas configuradas
  - `os`: Sistema operativo (linux)
  - `uptime_seconds`: Tiempo de actividad del sistema en segundos

### Métricas del Sistema
- **Endpoint**: `POST /v1/node-metrics/metrics`
- **Frecuencia**: Cada 15 segundos
- **Datos enviados**:
  - **CPU**: Porcentaje de uso
  - **Memoria**: Total, usado, disponible y porcentaje
  - **Disco**: Total, usado, disponible y porcentaje (partición raíz)
  - `disk_total_bytes`: Espacio total en disco en bytes
  - `disk_used_bytes`: Espacio usado en disco en bytes
  - `disk_available_bytes`: Espacio disponible en disco en bytes
  - `disk_usage_percent`: Porcentaje de uso de disco

## Próximas Funcionalidades

- Logs del sistema
- Ejecución de comandos remotos
- Actualizaciones automáticas

## Seguridad

- El token de API se almacena en `/etc/autohost/config.yaml`
- Asegúrate de que el archivo de configuración tenga permisos apropiados:
  ```bash
  sudo chmod 600 /etc/autohost/config.yaml
  ```
