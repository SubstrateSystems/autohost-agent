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

// DockerPgBackup implements CommandWithOutput to create a PostgreSQL logical backup
// using pg_dumpall (or pg_dump for a specific database) executed inside the container,
// then uploads the compressed SQL dump to S3 / R2.
type DockerPgBackup struct{}

type PgBackupPayload struct {
	ContainerName string   `json:"container_name"`
	PgUser        string   `json:"pg_user"`
	Database      string   `json:"database"` // empty = pg_dumpall
	S3            S3Config `json:"s3"`
}

type PgBackupResult struct {
	S3Key         string `json:"s3_key"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	DurationMS    int64  `json:"duration_ms"`
}

func (c *DockerPgBackup) Execute(ctx context.Context, payload map[string]any) error {
	_, err := c.ExecuteWithOutput(ctx, payload)
	return err
}

func (c *DockerPgBackup) ExecuteWithOutput(ctx context.Context, payload map[string]any) (string, error) {
	startTime := time.Now()

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req PgBackupPayload
	if err := json.Unmarshal(data, &req); err != nil {
		return "", fmt.Errorf("failed to parse pg backup payload: %w", err)
	}

	if req.ContainerName == "" {
		return "", fmt.Errorf("container_name is required")
	}
	if !safeContainerName.MatchString(req.ContainerName) {
		return "", fmt.Errorf("invalid container name: %q", req.ContainerName)
	}
	if req.S3.Bucket == "" || req.S3.Endpoint == "" || req.S3.AccessKeyID == "" {
		return "", fmt.Errorf("invalid S3/R2 storage configuration")
	}

	pgUser := req.PgUser
	if pgUser == "" {
		pgUser = "postgres"
	}

	// Create temp file for the compressed SQL dump
	tmpFile, err := os.CreateTemp("", "autohost-pgbackup-*.sql.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	dumpFilePath := tmpFile.Name()
	defer os.Remove(dumpFilePath)
	defer tmpFile.Close()

	// Build the docker exec command:
	// For a specific database: docker exec <container> pg_dump -U <user> <database> | gzip
	// For all databases:       docker exec <container> pg_dumpall -U <user> | gzip
	//
	// We pipe through gzip via shell to compress the output.
	var dumpCmd string
	if req.Database != "" {
		dumpCmd = fmt.Sprintf("pg_dump -U %s %s", pgUser, req.Database)
	} else {
		dumpCmd = fmt.Sprintf("pg_dumpall -U %s", pgUser)
	}

	// Execute: docker exec <container> sh -c '<dumpCmd>' | gzip > tmpFile
	// We use sh -c on the host side to pipe gzip
	shellCmd := fmt.Sprintf("docker exec %s sh -c '%s' | gzip", req.ContainerName, dumpCmd)
	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	cmd.Stdout = tmpFile
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pg_dump failed: %s (%w)", errBuf.String(), err)
	}
	_ = tmpFile.Close()

	fi, err := os.Stat(dumpFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat dump file: %w", err)
	}
	fileSizeBytes := fi.Size()

	// Sanity check: pg_dump with errors can still produce a tiny output
	if fileSizeBytes < 50 {
		return "", fmt.Errorf("pg_dump produced a suspiciously small output (%d bytes), possible error in dump", fileSizeBytes)
	}

	s3Key := req.S3.S3Key
	if s3Key == "" {
		s3Key = fmt.Sprintf("backups/%s/%s_%s.sql.gz", req.ContainerName, req.ContainerName, time.Now().Format("20060102_150405"))
	}

	if err := uploadToS3(ctx, req.S3, dumpFilePath, s3Key); err != nil {
		return "", fmt.Errorf("S3 upload failed: %w", err)
	}

	duration := time.Since(startTime).Milliseconds()

	result := PgBackupResult{
		S3Key:         s3Key,
		FileSizeBytes: fileSizeBytes,
		DurationMS:    duration,
	}

	resBytes, _ := json.Marshal(result)
	return string(resBytes), nil
}
