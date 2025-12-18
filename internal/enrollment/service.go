package enrollment

import (
	"context"
)

// Service handles agent enrollment operations.
type Service struct {
	apiURL string
}

// NewService creates a new enrollment service.
func NewService(apiURL string) *Service {
	return &Service{
		apiURL: apiURL,
	}
}

// Enroll registers a new agent with the backend.
func (s *Service) Enroll(ctx context.Context, token string) error {
	// TODO: Implementar lógica de enrollment
	// - Validar token
	// - Registrar agente
	// - Obtener credenciales
	// - Guardar configuración
	return nil
}
