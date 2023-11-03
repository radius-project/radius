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

	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// tfLogWrapper is a wrapper around the Terraform logger to stream the logs to the Radius logger.
type tfLogWrapper struct {
	logger   logr.Logger
	isStdErr bool
}

// Write implements the io.Writer interface to stream the Terraform logs to the Radius logger.
func (w *tfLogWrapper) Write(p []byte) (n int, err error) {
	if w.isStdErr {
		w.logger.Error(nil, string(p))
	} else {
		w.logger.Info(string(p))
	}

	return len(p), nil
}

// configureTerraformLogs configures the Terraform logs to be streamed to the Radius logs.
func configureTerraformLogs(ctx context.Context, tf *tfexec.Terraform) {
	logger := ucplog.FromContextOrDiscard(ctx)

	err := tf.SetLog("TRACE")
	if err != nil {
		logger.Error(err, "Failed to set log level for Terraform")
		return
	}

	tf.SetStdout(&tfLogWrapper{logger: logger})
	tf.SetStderr(&tfLogWrapper{logger: logger, isStdErr: true})
}
