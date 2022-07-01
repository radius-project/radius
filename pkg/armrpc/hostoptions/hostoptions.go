// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// hostoptions defines and reads options for the RP's execution environment.

package hostoptions

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/radrp/k8sauth"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// HostOptions defines all of the settings that our RP's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap configuration loaded from config file.
	Config    *ProviderConfig
	Arm       *armauth.ArmConfig
	K8sConfig *rest.Config
}

func NewHostOptionsFromEnvironment(configPath string) (HostOptions, error) {
	conf, err := loadConfig(configPath)
	if err != nil {
		return HostOptions{}, err
	}

	arm, err := getArm()
	if err != nil {
		return HostOptions{}, err
	}

	k8s, err := getKubernetes()
	if err != nil {
		return HostOptions{}, err
	}

	return HostOptions{
		Config:    conf,
		Arm:       arm,
		K8sConfig: k8s,
	}, nil
}

func loadConfig(configPath string) (*ProviderConfig, error) {
	buf, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	conf := &ProviderConfig{}
	decoder := yaml.NewDecoder(bytes.NewBuffer(buf))
	decoder.KnownFields(true)

	err = decoder.Decode(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load yaml: %w", err)
	}

	// TODO: improve the way to override the configration via env var.
	cosmosdbUrl := os.Getenv("RADIUS_STORAGEPROVIDER_COSMOSDB_URL")
	if cosmosdbUrl != "" {
		conf.StorageProvider.CosmosDB.Url = cosmosdbUrl
	}

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

func getArm() (*armauth.ArmConfig, error) {
	skipARM, ok := os.LookupEnv("SKIP_ARM")
	if ok && strings.EqualFold(skipARM, "true") {
		return nil, nil
	}

	arm, err := armauth.GetArmConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build ARM config: %w", err)
	}

	if arm != nil {
		fmt.Println("Initializing RP with the provided ARM credentials")
	}

	return arm, nil
}

func getKubernetes() (*rest.Config, error) {
	cfg, err := k8sauth.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Verify that we can connect to the cluster before handing out the config
	s := scheme.Scheme
	c, err := controller_runtime.New(cfg, controller_runtime.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := &corev1.NamespaceList{}
	err = c.List(context.Background(), ns, &controller_runtime.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes: %w", err)
	}

	return cfg, nil
}
