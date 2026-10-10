/*
Copyright 2026 The Radius Authors.

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

package authz

import (
	"fmt"

	"github.com/go-logr/logr"
	"go.yaml.in/yaml/v3"
)

// Mode is the authorization mode of a Radius installation.
type Mode string

const (
	// ModeOff disables authorization checks. This is the default.
	ModeOff Mode = "off"

	// ModeDryRun runs authorization checks and logs denials without rejecting requests.
	ModeDryRun Mode = "dryRun"

	// ModeEnforce runs authorization checks and rejects denied requests.
	ModeEnforce Mode = "enforce"
)

// ParseMode parses an authorization mode. An empty string is ModeOff. Values are case-sensitive.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeDryRun, ModeEnforce:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("invalid authorization mode %q: must be one of %q, %q, or %q", s, ModeOff, ModeDryRun, ModeEnforce)
	}
}

// UnmarshalYAML parses and validates a Mode from configuration.
func (m *Mode) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}

	parsed, err := ParseMode(s)
	if err != nil {
		return err
	}

	*m = parsed
	return nil
}

// Options is the authorization configuration shared by Radius services.
type Options struct {
	// Mode is the authorization mode. Defaults to ModeOff when unset.
	Mode Mode `yaml:"mode,omitempty"`
}

// EffectiveMode returns the configured mode, or ModeOff when it is unset.
func (o Options) EffectiveMode() Mode {
	if o.Mode == "" {
		return ModeOff
	}

	return o.Mode
}

// LogMode logs the effective authorization mode. Services call it once at startup.
func LogMode(logger logr.Logger, options Options) {
	mode := options.EffectiveMode()
	logger.Info(fmt.Sprintf("authz mode=%s", mode), LogFieldMode, string(mode))
}
