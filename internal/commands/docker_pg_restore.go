package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// DockerPgRestore implements CommandWithOutput to restore a PostgreSQL logical backup
// by downloading the compressed SQL dump from S3 / R2 and piping it into psql.
type DockerPgRestore struct{}

type PgRestorePayload struct {
	ContainerName string   `json:"container_name"`
	PgUser        string   `json:"pg_user"`
	Database      string   `json:"database"` // empty = restore to default (used with pg_dumpall output)
	S3            S3Config `json:"s3"`
}

type PgRestoreResult struct {
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
}

func (c *DockerPgRestore) Execute(ctx context.Context, payload map[string]any) error {
	_, err := c.ExecuteWithOutput(ctx, payload)
	return err
}

func (c *DockerPgRestore) ExecuteWithOutput(ctx context.Context, payload map[string]any) (string, error) {
	startTime := time.Now()

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req PgRestorePayload
	if err := json.Unmarshal(data, &req); err != nil {
		return "", fmt.Errorf("failed to parse pg restore payload: %w", err)
	}

	if req.ContainerName == "" {
		return "", fmt.Errorf("container_name is required")
	}
	if !safeContainerName.MatchString(req.ContainerName) {
		return "", fmt.Errorf("invalid container name: %q", req.ContainerName)
	}
	if req.S3.Bucket == "" || req.S3.Endpoint == "" || req.S3.S3Key == "" {
		return "", fmt.Errorf("invalid S3/R2 storage configuration or s3_key missing")
	}

	pgUser := req.PgUser
	if pgUser == "" {
		pgUser = "postgres"
	}

	// Download the SQL dump from S3
	tmpFile, err := os.CreateTemp("", "autohost-pgrestore-*.sql.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	dumpFilePath := tmpFile.Name()
	defer os.Remove(dumpFilePath)
	_ = tmpFile.Close()

	if err := downloadFromS3(ctx, req.S3, dumpFilePath); err != nil {
		return "", fmt.Errorf("S3 download failed: %w", err)
	}

	// Restore: gunzip the dump and pipe into psql running inside the container.
	// For pg_dumpall output, we always restore into the default database target (psql handles it).
	// For single-db dumps, we target the specific database.
	//
	// Command: gunzip -c <file> | docker exec -i <container> psql -U <user> [<database>]
	var psqlTarget string
	if req.Database != "" {
		psqlTarget = req.Database
	}

	var shellCmd string
	if psqlTarget != "" {
		shellCmd = fmt.Sprintf("gunzip -c %s | docker exec -i %s psql -U %s %s", dumpFilePath, req.ContainerName, pgUser, psqlTarget)
	} else {
		shellCmd = fmt.Sprintf("gunzip -c %s | docker exec -i %s psql -U %s", dumpFilePath, req.ContainerName, pgUser)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	// Discard stdout (psql output can be verbose)
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql restore failed: %s (%w)", errBuf.String(), err)
	}

	duration := time.Since(startTime).Milliseconds()
	result := PgRestoreResult{
		Status:     "completed",
		DurationMS: duration,
	}
	resBytes, _ := json.Marshal(result)
	return string(resBytes), nil
}
