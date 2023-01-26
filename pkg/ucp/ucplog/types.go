// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

// LoggingOptions represents the logger.
type LoggingOptions struct {
	Json  bool   `yaml:"json"`
	Level string `yaml:"level"`
}
