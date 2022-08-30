// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"github.com/spf13/viper"
)

type ConfigHolder struct {
	ConfigFilePath string
	Config         *viper.Viper
}

func NewConfigHolder() *ConfigHolder {
	return &ConfigHolder{}
}
