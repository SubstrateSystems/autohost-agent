package shell

import (
	"os"
	"os/exec"
)

func Exec(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
func ExecShell(script string) error {
	// bash con -e (stop on error) y -o pipefail
	return Exec("bash", "-eo", "pipefail", "-c", script)
}

func ExecWithDir(dir string, cmdName string, args ...string) error {
	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
