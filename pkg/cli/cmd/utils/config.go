// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package utils

import (
	"context"

	"github.com/spf13/viper"
)

type contextKey string

type ConfigInterface interface {
	ConfigFromContext(ctx context.Context) *viper.Viper
}

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

var _ ConfigInterface = (*ConfigHolder)(nil)

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
