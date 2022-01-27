// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package k3d

import (
	"context"
	"os"
	"os/exec"
)

func DeleteCluster(ctx context.Context, name string) error {
	err := RequireK3dInstalled()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "k3d", "cluster", "delete", name)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Start()
	if err != nil {
		return err
	}

	// We don't need to show anything interactive here, the k3d binary outputs progress
	// messages.

	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}
