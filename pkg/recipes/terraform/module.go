/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package terraform

import (
	"context"

	"github.com/hashicorp/terraform-exec/tfexec"
)

const (
	moduleRootDir = ".terraform/modules"
)

// downloadModule downloads the module to the working directory from the module source specified in the Terraform configuration.
// It uses Terraform's Get command to download the module.
// Input parameters:
// workingDir is the working directory of the Terraform configuration.
// execPath is the path to the Terraform executable.
func downloadModule(ctx context.Context, workingDir, execPath string) error {
	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return err
	}

	if err = tf.Get(ctx); err != nil {
		return err
	}

	return nil
}
