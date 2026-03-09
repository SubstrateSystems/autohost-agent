package dir

import (
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
