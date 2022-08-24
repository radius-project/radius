// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package shared

import (
	"context"

	"github.com/spf13/viper"
)

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

type contextKey string

type ConfigHolder struct {
	ConfigFilePath string
	Config         *viper.Viper
}

func NewConfigHolder() *ConfigHolder {
	return &ConfigHolder{}
}

func (c *ConfigHolder) ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(NewContextKey("config")).(*ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}
