# Autohost Agent

Agent de monitorización para Linux que reporta el estado del nodo a una API central.

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
curl -fsSL https://raw.githubusercontent.com/SubstrateSystems/autohost-agent/main/scripts/install.sh | bash
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
bash <(curl -fsSL https://raw.githubusercontent.com/SubstrateSystems/autohost-agent/main/scripts/install.sh)
```

### Versión específica

```bash
VERSION=v0.2.0 curl -fsSL https://raw.githubusercontent.com/SubstrateSystems/autohost-agent/main/scripts/install.sh | bash
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
