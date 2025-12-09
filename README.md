# Autohost Agent

Agent de monitorización para Linux que reporta el estado del nodo a una API central.

## Estructura del Proyecto

```
autohost-agent/
├── cmd/
│   └── autohost-agent/     # Punto de entrada principal
│       └── main.go
├── internal/
│   ├── agent/              # Lógica principal del agente
│   │   └── agent.go
│   ├── cloud/              # Cliente HTTP para la API
│   │   └── client.go
│   ├── config/             # Configuración del agente
│   │   └── config.go
│   └── system/             # Utilidades del sistema (uptime, etc)
│       └── uptime.go
├── config.example.yaml     # Ejemplo de configuración
└── go.mod
```

## Configuración

1. Copia el archivo de configuración de ejemplo:
```bash
cp config.example.yaml /etc/autohost/config.yaml
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
go build -o autohost-agent cmd/autohost-agent/main.go
```

## Ejecución

### Modo manual (desarrollo)
```bash
./autohost-agent /etc/autohost/config.yaml
```

### Como servicio systemd (producción)

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

## Próximas Funcionalidades

- Métricas de sistema (CPU, memoria, disco)
- Logs del sistema
- Ejecución de comandos remotos
- Actualizaciones automáticas

## Seguridad

- El token de API se almacena en `/etc/autohost/config.yaml`
- Asegúrate de que el archivo de configuración tenga permisos apropiados:
  ```bash
  sudo chmod 600 /etc/autohost/config.yaml
  ```
