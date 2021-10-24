// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"
)

func Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wd, err := GetLocalWorkingDirectory()
	if err != nil {
		return err
	}

	err = os.MkdirAll(wd, os.FileMode(0755))
	if err != nil {
		return err
	}

	kcp, err := startKCP(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to start kcp: %w", err)
	}

	err = applyCRDsWithRetries(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to apply CRDs: %w", err)
	}

	radiusd, err := startRadiusD(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to start radiusd: %w", err)
	}

	controller, err := startRadiusController(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to start radius-controller: %w", err)
	}

	err = controller.Wait()
	if err != nil {
		return fmt.Errorf("failed to run radius-controller: %w", err)
	}

	err = radiusd.Wait()
	if err != nil {
		return fmt.Errorf("failed to run radiusd: %w", err)
	}

	err = kcp.Wait()
	if err != nil {
		return fmt.Errorf("failed to run kcp: %w", err)
	}

	return nil
}

func startKCP(ctx context.Context, wd string) (*exec.Cmd, error) {
	executable, err := GetLocalKCPFilepath()
	if err != nil {
		return nil, err
	}

	args := []string{
		"start",
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = wd
	// Don't log stdout/stderr for now, it's really spammy.
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func startRadiusD(ctx context.Context, wd string) (*exec.Cmd, error) {
	executable, err := GetLocalRadiusDFilepath()
	if err != nil {
		return nil, err
	}

	kubeConfigPath := path.Join(wd, ".kcp", "data", "admin.kubeconfig")
	args := []string{
		"-zap-devel",
		"--kubeconfig", kubeConfigPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func startRadiusController(ctx context.Context, wd string) (*exec.Cmd, error) {
	executable, err := GetLocalRadiusControllerFilepath()
	if err != nil {
		return nil, err
	}

	kubeConfigPath := path.Join(wd, ".kcp", "data", "admin.kubeconfig")
	args := []string{
		"-zap-devel",
		"-model", "local",
		"--kubeconfig", kubeConfigPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = []string{"SKIP_WEBHOOKS=true"}
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func applyCRDsWithRetries(ctx context.Context, wd string) error {
	var err error
	for i := 0; i < 20; i++ {
		err = applyCRDs(ctx, wd)
		if err == nil {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return err
}

func applyCRDs(ctx context.Context, wd string) error {
	executable := "kubectl"

	kubeConfigPath := path.Join(wd, ".kcp", "data", "admin.kubeconfig")
	args := []string{
		"apply",
		"-f", "../crd/",
		"--kubeconfig", kubeConfigPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	args = []string{
		"wait",
		"--for", "condition=established",
		"-f", "../crd/",
		"--kubeconfig", kubeConfigPath,
	}
	cmd = exec.CommandContext(ctx, executable, args...)
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
