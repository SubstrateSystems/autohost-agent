package assets

import (
	"embed"
	"io/fs"
	"path/filepath"
)

//go:embed docker/**/*
var dockerFS embed.FS

// ReadCompose lee assets/docker/<app>/docker-compose.yml
func ReadCompose(app string) ([]byte, error) {
	return fs.ReadFile(dockerFS, filepath.Join("docker", app, "compose.yml"))
}

func ReadEnvExample(app string) ([]byte, error) {
	return fs.ReadFile(dockerFS, filepath.Join("docker", app, ".env.example"))
}
