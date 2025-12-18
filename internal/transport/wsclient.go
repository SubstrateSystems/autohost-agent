package transport

import (
	"context"
	"log"
)

// WSClient handles WebSocket communication with the backend.
type WSClient struct {
	url   string
	token string
}

// NewWSClient creates a new WebSocket client.
func NewWSClient(url, token string) *WSClient {
	return &WSClient{
		url:   url,
		token: token,
	}
}

// Connect establishes a WebSocket connection.
func (c *WSClient) Connect(ctx context.Context) error {
	// TODO: Implementar conexión WebSocket
	// - Conectar al servidor
	// - Autenticar
	// - Mantener conexión activa
	// - Recibir comandos en tiempo real
	log.Println("WebSocket client: connection not implemented yet")
	return nil
}

// Close closes the WebSocket connection.
func (c *WSClient) Close() error {
	// TODO: Cerrar conexión limpiamente
	return nil
}
