package jobs

import (
	"context"
	"fmt"
	"log"

	"autohost-agent/internal/adapters/docker"
	"autohost-agent/internal/agent/actions/app"
	"autohost-agent/internal/domain"
)

// Runner executes jobs received from the backend.
type Runner struct {
	// Add dependencies as needed
}

// NewRunner creates a new job runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Execute runs a job and returns the result.
func (r *Runner) Execute(ctx context.Context, job *Job) error {
	log.Printf("Executing job: %s (type: %s)", job.ID, job.Type)

	job.Status = JobStatusRunning

	var err error
	switch job.Type {
	case "app.start":
		err = r.executeAppStart(job)
	case "app.stop":
		err = r.executeAppStop(job)
	case "app.restart":
		err = r.executeAppRestart(job)
	case "app.remove":
		err = r.executeAppRemove(job)
	case "docker.install":
		err = r.executeDockerInstall(job)
	// case "docker.check":
	// err = r.executeDockerCheck(job)
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}

	if err != nil {
		job.Status = JobStatusFailed
		return err
	}

	job.Status = JobStatusCompleted
	return nil
}

func (r *Runner) executeAppStart(job *Job) error {
	appName, ok := job.Payload["app_name"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid app_name")
	}
	return app.Start(domain.AppName(appName))
}

func (r *Runner) executeAppStop(job *Job) error {
	appName, ok := job.Payload["app_name"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid app_name")
	}
	return app.Stop(domain.AppName(appName))
}

func (r *Runner) executeAppRestart(job *Job) error {
	appName, ok := job.Payload["app_name"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid app_name")
	}
	return app.Restart(domain.AppName(appName))
}

func (r *Runner) executeAppRemove(job *Job) error {
	appName, ok := job.Payload["app_name"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid app_name")
	}
	return app.Remove(domain.AppName(appName))
}

func (r *Runner) executeDockerInstall(job *Job) error {
	log.Println("Installing Docker...")
	return docker.Install()
}

// func (r *Runner) executeDockerCheck(job *Job) error {
// 	installed, err := docker.IsInstalled()
// 	if err != nil {
// 		return err
// 	}
// 	if !installed {
// 		return fmt.Errorf("docker is not installed")
// 	}
// 	log.Println("Docker is installed and running")
// 	return nil
// }
