// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/marstr/randname"
	"github.com/project-radius/radius/pkg/radlogger"
)

// GenerateRandomName generates a string with the specified prefix and a random 5-character suffix.
func GenerateRandomName(prefix string, affixes ...string) string {
	b := bytes.NewBufferString(prefix)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

// generateUniqueAzureResourceName generates a name with the specified prefix appended with -<uuid>.
// Uniqueness is validated based on the function provided for checking availability of the generated name.
// This is useful for Azure resource types where the resource names are required to be unique across Azure.
func generateUniqueAzureResourceName(ctx context.Context, prefix string, checkAvailability func(string) error) (string, error) {
	logger, err := radlogger.GetLogger(ctx)
	if err != nil {
		return "", err
	}
	retryAttempts := 10

	base := prefix + "-"

	for i := 0; i < retryAttempts; i++ {
		// 3-24 characters - all alphanumeric and '-'
		uid, err := uuid.NewV4()
		if err != nil {
			return "", fmt.Errorf("failed to generate name: %w", err)
		}
		name := base + strings.ReplaceAll(uid.String(), "-", "")
		name = name[0:24]
		err = checkAvailability(name)
		if err == nil {
			return name, nil
		}

		logger.Info(fmt.Sprintf("name generation failed after %d attempts", i))
	}

	return "", fmt.Errorf("name generation failed to create a unique name after %d attempts", retryAttempts)
}
