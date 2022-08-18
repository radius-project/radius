// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package utils

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/viper"
)

type contextKey string

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

type ConfigHolder struct {
	ConfigFilePath string
	Config         *viper.Viper
}

func NewConfigHolder() *ConfigHolder {
	return &ConfigHolder{}
}

func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(NewContextKey("config")).(*ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}
