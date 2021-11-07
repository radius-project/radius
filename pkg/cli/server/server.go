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
)

type Options struct {
	Clean bool
}

func Run(ctx context.Context, options Options) error {
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

	radiusd, err := startRadiusD(ctx, wd, options.Clean)
	if err != nil {
		return fmt.Errorf("failed to start radiusd: %w", err)
	}

	err = radiusd.Wait()
	if err != nil {
		return fmt.Errorf("failed to run radiusd: %w", err)
	}

	return nil
}

func startRadiusD(ctx context.Context, wd string, clean bool) (*exec.Cmd, error) {
	executable, err := GetLocalRadiusDFilepath()
	if err != nil {
		return nil, err
	}

	args := []string{}
	if clean {
		args = append(args, "--clean")
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
