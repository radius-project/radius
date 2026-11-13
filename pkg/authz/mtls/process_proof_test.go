/*
Copyright 2026 The Radius Authors.

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

package mtls

import (
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestProcessStartupProofRejectsLegacyConfiguration(t *testing.T) {
	for _, enabled := range []string{"", "false", "true", "invalid"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("UCP_MTLS_STARTUP_PROOF_ENABLED", enabled)
			t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", "")
			t.Setenv("UCP_MTLS_POD_UID", "")
			proof, err := ProcessStartupProof(authz.Options{}, true)
			require.Nil(t, proof)
			if enabled == "" || enabled == "false" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			t.Setenv("UCP_MTLS_EXPECTED_TRUST_SHA256", "orphan-setting")
			_, err = ProcessStartupProof(authz.Options{}, true)
			require.Error(t, err)
		})
	}
}
