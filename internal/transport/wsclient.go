package transport

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"autohost-agent/internal/jobs"

	"github.com/gorilla/websocket"
)

// WSClient handles WebSocket communication with the backend.
type WSClient struct {
	url    string
	token  string
	conn   *websocket.Conn
	mu     sync.Mutex
	runner *jobs.Runner
}

// NewWSClient creates a new WebSocket client.
func NewWSClient(url, token string, runner *jobs.Runner) *WSClient {
	return &WSClient{
		url:    url,
		token:  token,
		runner: runner,
	}
}

// Connect establishes a WebSocket connection and handles reconnection.
func (c *WSClient) Connect(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.connectOnce(ctx); err != nil {
				log.Printf("WebSocket connection failed: %v, retrying in 10s...", err)
				time.Sleep(10 * time.Second)
				continue
			}
		}
	}
}

// connectOnce attempts to connect once and handle messages.
func (c *WSClient) connectOnce(ctx context.Context) error {
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+c.token)

	log.Printf("Connecting to WebSocket: %s", c.url)
	conn, _, err := websocket.DefaultDialer.Dial(c.url, headers)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Println("WebSocket connected successfully")

	// Enviar mensaje de identificación
	if err := c.sendIdentification(); err != nil {
		log.Printf("Failed to send identification: %v", err)
		return err
	}

	// Leer mensajes en loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				return err
			}
			go c.handleMessage(ctx, message)
		}
	}
}

// sendIdentification sends initial identification message.
func (c *WSClient) sendIdentification() error {
	msg := map[string]interface{}{
		"type": "identify",
		"data": map[string]string{
			"version": "1.0.0",
		},
	}
	return c.send(msg)
}

// handleMessage processes incoming messages from the server.
func (c *WSClient) handleMessage(ctx context.Context, message []byte) {
	log.Printf("Received message: %s", string(message))

	var job jobs.Job
	if err := json.Unmarshal(message, &job); err != nil {
		log.Printf("Failed to unmarshal job: %v", err)
		return
	}

	// Ejecutar el job
	if err := c.runner.Execute(ctx, &job); err != nil {
		log.Printf("Job execution failed: %v", err)
		c.sendJobResult(&job, "failed", err.Error())
	} else {
		log.Printf("Job completed successfully: %s", job.ID)
		c.sendJobResult(&job, "completed", "")
	}
}

// sendJobResult sends the job execution result back to the server.
func (c *WSClient) sendJobResult(job *jobs.Job, status, errorMsg string) {
	result := map[string]interface{}{
		"type":         "job_result",
		"job_id":       job.ID,
		"status":       status,
		"error":        errorMsg,
		"completed_at": time.Now().Unix(),
	}

	if err := c.send(result); err != nil {
		log.Printf("Failed to send job result: %v", err)
	}
}

// send sends a JSON message through the WebSocket connection.
func (c *WSClient) send(msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil // Connection not ready
	}

	return c.conn.WriteJSON(msg)
}

// Close closes the WebSocket connection.
func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
