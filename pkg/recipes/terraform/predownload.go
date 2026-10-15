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
	"os"
	"strconv"
	"time"

	install "github.com/hashicorp/hc-install"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	preDownloadMaxAttempts  = 5
	preDownloadInitialDelay = 5 * time.Second
	preDownloadMaxDelay     = time.Minute
)

// PreDownloadEnvVar enables PreDownloadService when set to a true value. An environment variable is
// used rather than a config file field so that newer Helm charts remain compatible with older images,
// which reject unknown config fields.
const PreDownloadEnvVar = "RADIUS_TERRAFORM_PREDOWNLOAD"

// PreDownloadEnabled reports whether Terraform should be downloaded in the background at startup.
func PreDownloadEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv(PreDownloadEnvVar))
	return err == nil && enabled
}

// PreDownloadService downloads the global shared Terraform binary in the background when the
// process starts so that the first Terraform recipe does not pay the download cost.
//
// Downloading in-process (rather than in an init container) lets the pod become ready without
// waiting for the download, and uses hc-install, which verifies the release checksum and signature.
// Failures are logged and never returned: recipes fall back to downloading Terraform on demand.
type PreDownloadService struct {
	// ensure installs the binary. Defaults to ensureGlobalTerraformBinary; overridden in tests.
	ensure func(ctx context.Context) error

	// initialDelay is the delay before the first retry. Defaults to preDownloadInitialDelay.
	initialDelay time.Duration
}

// NewPreDownloadService creates a new PreDownloadService.
func NewPreDownloadService() *PreDownloadService {
	return &PreDownloadService{}
}

// Name implements hosting.Service.
func (s *PreDownloadService) Name() string {
	return "terraform pre-download"
}

// Run implements hosting.Service.
func (s *PreDownloadService) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	ensure := s.ensure
	if ensure == nil {
		ensure = func(ctx context.Context) error {
			_, err := ensureGlobalTerraformBinary(ctx, install.NewInstaller(), logger)
			return err
		}
	}

	delay := s.initialDelay
	if delay == 0 {
		delay = preDownloadInitialDelay
	}

	for attempt := 1; ; attempt++ {
		err := ensure(ctx)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return nil
		}

		if attempt >= preDownloadMaxAttempts {
			logger.Error(err, "Failed to pre-download Terraform; it will be downloaded when the first Terraform recipe runs", "attempts", attempt)
			return nil
		}

		logger.Error(err, "Failed to pre-download Terraform, retrying", "attempt", attempt, "delay", delay.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		delay = min(delay*2, preDownloadMaxDelay)
	}
}
