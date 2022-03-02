// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package k3d

import (
	"errors"
	"fmt"
	"os/exec"
)

func RequireK3dInstalled() error {
	_, err := exec.LookPath("k3d")
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("could not find k3d on the system path. Follow installation instructions at: https://k3d.io/#installation")
	} else if err != nil {
		return fmt.Errorf("failed to find k3d: %w", err)
	}

	return nil
}
