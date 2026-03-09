package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func runShell(command, dir string, stdin io.Reader, stdout, stderr io.Writer, env []string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), env...)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("command failed with exit code %d", exitErr.ExitCode())
		}
		return err
	}
	return nil
}
