package jobs

import (
	"context"
	"log"
)

// Runner executes jobs received from the backend.
type Runner struct {
	// TODO: Agregar dependencias necesarias
}

// NewRunner creates a new job runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Execute runs a job and returns the result.
func (r *Runner) Execute(ctx context.Context, job *Job) error {
	log.Printf("Executing job: %s (type: %s)", job.ID, job.Type)

	// TODO: Implementar ejecución de trabajos según el tipo:
	// - Ejecutar comandos
	// - Actualizar configuración
	// - Reiniciar servicios
	// - Recolectar logs
	// etc.

	return nil
}
