// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// hostoptions defines and reads options for the RP's execution environment.

package hostoptions

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"gopkg.in/yaml.v3"
)

// HostOptions defines all of the settings that our RP's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap configuration loaded from config file.
	Config *ProviderConfig

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

func loadConfig(configPath string) (*ProviderConfig, error) {
	buf, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	conf := &ProviderConfig{}
	err = yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, fmt.Errorf("fails to load yaml: %w", err)
	}

	// TODO: improve the way to override the configration via env var.
	cosmosDBKey := os.Getenv("RADIUS_STORAGEPROVIDER_COSMOSDB_MASTERKEY")
	if cosmosDBKey != "" {
		conf.StorageProvider.CosmosDB.MasterKey = cosmosDBKey
	}

	return conf, nil
}

// FromContext extracts ProviderConfig from http context.
func FromContext(ctx context.Context) *ProviderConfig {
	return ctx.Value(servicecontext.HostingConfigContextKey).(*ProviderConfig)
}

// WithContext injects ProviderConfig into the given http context.
func WithContext(ctx context.Context, cfg *ProviderConfig) context.Context {
	return context.WithValue(ctx, servicecontext.HostingConfigContextKey, cfg)
}
