package localenv

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func ApplyCRDs(ctx context.Context, kubeConfigPath string, directory string) {
	applyCRDsWithRetries(ctx, kubeConfigPath, directory)
}

func applyCRDsWithRetries(ctx context.Context, kubeConfigPath string, directory string) error {
	var err error
	for i := 0; i < 20; i++ {
		err = applyCRDs(ctx, kubeConfigPath, directory)
		if err == nil {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return err
}

func applyCRDs(ctx context.Context, kubeConfigPath string, crdDirectory string) error {
	executable := "kubectl"

	args := []string{
		"apply",
		"-f", crdDirectory,
		"--kubeconfig", kubeConfigPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	args = []string{
		"wait",
		"--for", "condition=established",
		"-f", crdDirectory,
		"--kubeconfig", kubeConfigPath,
	}
	cmd = exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
