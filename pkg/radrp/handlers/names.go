// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/gofrs/uuid"
)

// generateRandomAzureName generates account name with the specified database name as prefix appended with -<uuid>.
// This is needed since CosmosDB account names are required to be unique across Azure.
func generateRandomAzureName(ctx context.Context, nameBase string, checkAvailability func(string) error) (string, error) {

	logger := radlogger.GetLogger(ctx)
	retryAttempts := 10

	base := nameBase + "-"

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
