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
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func Test_writeTLSCertificates(t *testing.T) {
	tests := []struct {
		name    string
		tls     *recipes.TLSConfig
		secrets map[string]recipes.SecretData
		wantErr bool
		verify  func(t *testing.T, paths *tlsCertificatePaths, workingDir string)
	}{
		{
			name: "nil TLS config",
			tls:  nil,
			verify: func(t *testing.T, paths *tlsCertificatePaths, workingDir string) {
				require.Nil(t, paths)
			},
		},
		{
			name: "skip verify only without certificates",
			tls: &recipes.TLSConfig{
				SkipVerify: true,
			},
			verify: func(t *testing.T, paths *tlsCertificatePaths, workingDir string) {
				require.Nil(t, paths)
				// Verify no .tls directory was created
				tlsDir := filepath.Join(workingDir, ".tls")
				_, err := os.Stat(tlsDir)
				require.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "CA certificate only",
			tls: &recipes.TLSConfig{
				CACertificate: &recipes.SecretReference{
					Source: "/secrets/ca",
					Key:    "cert",
				},
			},
			secrets: map[string]recipes.SecretData{
				"/secrets/ca": {
					Data: map[string]string{
						"cert": "-----BEGIN CERTIFICATE-----\nMIIDQTCCAimgAwIBAgITBmyfz5m...\n-----END CERTIFICATE-----",
					},
				},
			},
			verify: func(t *testing.T, paths *tlsCertificatePaths, workingDir string) {
				require.NotNil(t, paths)
				require.NotEmpty(t, paths.CAPath)
				require.FileExists(t, paths.CAPath)

				content, err := os.ReadFile(paths.CAPath)
				require.NoError(t, err)
				require.Contains(t, string(content), "BEGIN CERTIFICATE")
			},
		},
		{
			name: "client certificates for mTLS",
			tls: &recipes.TLSConfig{
				ClientCertificate: &recipes.ClientCertConfig{
					Secret: "/secrets/client",
				},
			},
			secrets: map[string]recipes.SecretData{
				"/secrets/client": {
					Data: map[string]string{
						"certificate": "-----BEGIN CERTIFICATE-----\nMIIDQTCCAimgAwIBAgITBmyfz5m...\n-----END CERTIFICATE-----",
						"key":         "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQE...\n-----END PRIVATE KEY-----",
					},
				},
			},
			verify: func(t *testing.T, paths *tlsCertificatePaths, workingDir string) {
				require.NotNil(t, paths)
				require.NotEmpty(t, paths.ClientCertPath)
				require.NotEmpty(t, paths.ClientKeyPath)
				require.FileExists(t, paths.ClientCertPath)
				require.FileExists(t, paths.ClientKeyPath)

				// Verify file permissions
				info, err := os.Stat(paths.ClientKeyPath)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), info.Mode().Perm())
			},
		},
		{
			name: "missing secret source",
			tls: &recipes.TLSConfig{
				CACertificate: &recipes.SecretReference{
					Source: "/secrets/missing",
					Key:    "cert",
				},
			},
			secrets: map[string]recipes.SecretData{},
			wantErr: true,
		},
		{
			name: "missing secret key",
			tls: &recipes.TLSConfig{
				CACertificate: &recipes.SecretReference{
					Source: "/secrets/ca",
					Key:    "missing",
				},
			},
			secrets: map[string]recipes.SecretData{
				"/secrets/ca": {
					Data: map[string]string{
						"cert": "data",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := t.TempDir()

			paths, err := writeTLSCertificates(context.Background(), workingDir, tt.tls, tt.secrets)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.verify != nil {
				tt.verify(t, paths, workingDir)
			}
		})
	}
}
