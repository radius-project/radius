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

package recipes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	kafkaSecretID = "/planes/radius/local/resourceGroups/default/providers/Radius.Security/secrets/kafkasecret"
	otherSecretID = "/planes/radius/local/resourceGroups/default/providers/Radius.Security/secrets/othersecret"
)

func Test_ResolveSecretWriteBacks_NoSecretOutputs(t *testing.T) {
	writeBacks, consumed, err := ResolveSecretWriteBacks(
		map[string]any{"host": "example"},
		map[string]any{"primaryConnectionString": "secret-value"},
		nil,
		[]string{kafkaSecretID},
	)
	require.NoError(t, err)
	require.Nil(t, writeBacks)
	require.Nil(t, consumed)
}

func Test_ResolveSecretWriteBacks_FromSecrets(t *testing.T) {
	writeBacks, consumed, err := ResolveSecretWriteBacks(
		map[string]any{"host": "example"},
		map[string]any{"primaryConnectionString": "secret-value"},
		map[string]map[string]string{
			"kafkasecret": {"connectionString": "primaryConnectionString"},
		},
		[]string{kafkaSecretID},
	)
	require.NoError(t, err)
	require.Equal(t, []SecretWriteBack{
		{SecretID: kafkaSecretID, Data: map[string]string{"connectionString": "secret-value"}},
	}, writeBacks)
	require.Contains(t, consumed, "primaryConnectionString")
}

func Test_ResolveSecretWriteBacks_FromValues(t *testing.T) {
	// The referenced module output is a non-sensitive value (not a secret); it should still be resolved.
	writeBacks, consumed, err := ResolveSecretWriteBacks(
		map[string]any{"endpoint": "kafka:9092"},
		map[string]any{},
		map[string]map[string]string{
			"kafkasecret": {"broker": "endpoint"},
		},
		[]string{kafkaSecretID},
	)
	require.NoError(t, err)
	require.Equal(t, []SecretWriteBack{
		{SecretID: kafkaSecretID, Data: map[string]string{"broker": "kafka:9092"}},
	}, writeBacks)
	require.Contains(t, consumed, "endpoint")
}

func Test_ResolveSecretWriteBacks_SecretsPreferredOverValues(t *testing.T) {
	writeBacks, _, err := ResolveSecretWriteBacks(
		map[string]any{"conn": "from-values"},
		map[string]any{"conn": "from-secrets"},
		map[string]map[string]string{
			"kafkasecret": {"connectionString": "conn"},
		},
		[]string{kafkaSecretID},
	)
	require.NoError(t, err)
	require.Equal(t, "from-secrets", writeBacks[0].Data["connectionString"])
}

func Test_ResolveSecretWriteBacks_NonStringCoerced(t *testing.T) {
	writeBacks, _, err := ResolveSecretWriteBacks(
		map[string]any{"port": 9092},
		map[string]any{},
		map[string]map[string]string{
			"kafkasecret": {"port": "port"},
		},
		[]string{kafkaSecretID},
	)
	require.NoError(t, err)
	require.Equal(t, "9092", writeBacks[0].Data["port"])
}

func Test_ResolveSecretWriteBacks_MultipleSecretsAndKeys(t *testing.T) {
	writeBacks, consumed, err := ResolveSecretWriteBacks(
		map[string]any{},
		map[string]any{
			"connOut": "conn-value",
			"userOut": "user-value",
			"passOut": "pass-value",
		},
		map[string]map[string]string{
			"othersecret": {"conn": "connOut"},
			"kafkasecret": {"username": "userOut", "password": "passOut"},
		},
		[]string{kafkaSecretID, otherSecretID},
	)
	require.NoError(t, err)
	// Deterministic ordering: secret names sorted (kafkasecret before othersecret).
	require.Equal(t, []SecretWriteBack{
		{SecretID: kafkaSecretID, Data: map[string]string{"username": "user-value", "password": "pass-value"}},
		{SecretID: otherSecretID, Data: map[string]string{"conn": "conn-value"}},
	}, writeBacks)
	require.Len(t, consumed, 3)
}

func Test_ResolveSecretWriteBacks_UnboundSecret_FailsClosed(t *testing.T) {
	_, _, err := ResolveSecretWriteBacks(
		map[string]any{},
		map[string]any{"primaryConnectionString": "secret-value"},
		map[string]map[string]string{
			"kafkasecret": {"connectionString": "primaryConnectionString"},
		},
		// The resource does not bind kafkasecret.
		[]string{otherSecretID},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not bind")
}

func Test_ResolveSecretWriteBacks_MissingOutput_FailsClosed(t *testing.T) {
	_, _, err := ResolveSecretWriteBacks(
		map[string]any{},
		map[string]any{},
		map[string]map[string]string{
			"kafkasecret": {"connectionString": "primaryConnectionString"},
		},
		[]string{kafkaSecretID},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "produced no output named")
}

func Test_ResolveSecretWriteBacks_InvalidBoundSecretID(t *testing.T) {
	_, _, err := ResolveSecretWriteBacks(
		map[string]any{},
		map[string]any{"primaryConnectionString": "secret-value"},
		map[string]map[string]string{
			"kafkasecret": {"connectionString": "primaryConnectionString"},
		},
		[]string{"not-a-resource-id"},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid bound secret ID")
}
