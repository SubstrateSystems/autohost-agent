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

1. Copia el archivo de configuración de ejemplo:
```bash
cp configs/agent.yaml /etc/autohost/config.yaml
```

2. Edita el archivo de configuración con tus valores:
```yaml
api_url: "https://api.tudominio.com"
agent_token: "tu-token-api"
node_id: "nombre-unico-del-nodo"
tags:
  - "etiqueta1"
  - "etiqueta2"
```

## Compilación

```bash
make build
# o directamente:
go build -o autohost-agent cmd/agent/main.go
```

## Ejecución

### Modo manual (desarrollo)
```bash
./autohost-agent /etc/autohost/config.yaml
```

### Como servicio systemd (producción)

#### Opción 1: Usar el script de instalación
```bash
make build
sudo ./scripts/install.sh
```

#### Opción 2: Instalación manual

1. Copia el binario:
```bash
sudo cp autohost-agent /usr/local/bin/
```

2. Copia el archivo de servicio:
```bash
sudo cp autohost-agent.service /etc/systemd/system/
```

3. Habilita e inicia el servicio:
```bash
sudo systemctl daemon-reload
sudo systemctl enable autohost-agent
sudo systemctl start autohost-agent
```

4. Verifica el estado:
```bash
sudo systemctl status autohost-agent
sudo journalctl -u autohost-agent -f
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
