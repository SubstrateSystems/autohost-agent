package jobs

// Job represents a task to be executed by the agent.
type Job struct {
	ID          string
	Type        string
	Payload     map[string]interface{}
	Status      string
	CreatedAt   int64
	CompletedAt int64
}

// JobStatus represents the execution status of a job.
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
)
