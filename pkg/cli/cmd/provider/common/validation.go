// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
)

func ValidateCloudProviderName(name string) error {
	if strings.EqualFold(name, "azure") {
		return nil
	}

	return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider type %q is not supported. Supported types: azure.", name)}
}
