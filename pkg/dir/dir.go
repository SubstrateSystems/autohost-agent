package dir

import (
	"os"
	"path/filepath"
)

func GetAutohostDir() string {
	return "/var/lib/autohost"
}

func GetSubdir(subdir string) string {
	return filepath.Join(GetAutohostDir(), subdir)
}

func GetRootAppDir() string {
	return filepath.Join("/opt/autohost")
}

func EnsureAutohostDirs() error {
	subdirs := []string{
		"config",
		"templates",
		"apps",
		"logs",
		"state",
		"backups",
		"config",
	}

	for _, sub := range subdirs {
		if err := os.MkdirAll(GetSubdir(sub), 0755); err != nil {
			return err
		}
	}
	return nil
}
