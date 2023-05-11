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

// hostoptions defines and reads options for the RP's execution environment.

package hostoptions

import (
	"bytes"
	"context"
	"fmt"
	"os"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"gopkg.in/yaml.v3"
)

// HostOptions defines all of the settings that our RP's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap configuration loaded from config file.
	Config *UCPConfig

	// DBClientFactory func(ctx context.Context) (*mongo.Database, error)

	// HealthChannels supports loosely-coupled communication between the Resource Provider
	// backend and the Health Monitor. This is part of the options for now because it's
	// based on in-memory communication.
	// HealthChannels healthcontract.HealthChannels
}

func NewHostOptionsFromEnvironment(configPath string) (HostOptions, error) {
	conf, err := loadConfig(configPath)
	if err != nil {
		return HostOptions{}, err
	}

	return HostOptions{
		Config: conf,
	}, nil
}

func loadConfig(configPath string) (*UCPConfig, error) {
	buf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	conf := &UCPConfig{}
	decoder := yaml.NewDecoder(bytes.NewBuffer(buf))
	decoder.KnownFields(true)

	err = decoder.Decode(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load yaml: %w", err)
	}

	return conf, nil
}

// FromContext extracts ProviderConfig from http context.
func FromContext(ctx context.Context) *UCPConfig {
	return ctx.Value(v1.HostingConfigContextKey).(*UCPConfig)
}

// WithContext injects ProviderConfig into the given http context.
func WithContext(ctx context.Context, cfg *UCPConfig) context.Context {
	return context.WithValue(ctx, v1.HostingConfigContextKey, cfg)
}
