package dir

import (
	"os"
	"path/filepath"
)

func GetAutohostDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// fallback razonable si falla
		return "/tmp/autohost"
	}
	return filepath.Join(home, ".autohost")
}

func GetSubdir(subdir string) string {
	return filepath.Join(GetAutohostDir(), subdir)
}

func GetRootAppDir() string {
	return filepath.Join("/opt/autohost")
}
