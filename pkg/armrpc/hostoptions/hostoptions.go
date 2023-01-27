// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// hostoptions defines and reads options for the RP's execution environment.

package hostoptions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/armauth"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/sdk"
	ucpapi "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	sprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// HostOptions defines all of the settings that our RP's execution environment provides.
type HostOptions struct {
	// Config is the bootstrap configuration loaded from config file.
	Config *ProviderConfig

	// Arm is the ARM authorization configuration.
	Arm *armauth.ArmConfig

	// K8sConfig is the Kubernetes configuration for communicating with the local
	// cluster.
	K8sConfig *rest.Config

	// UCPConnection is a connection to the UCP endpoint.
	UCPConnection sdk.Connection
}

func getArmConfig(cfg *ProviderConfig, ucpconn sdk.Connection) (*armauth.ArmConfig, error) {
	skipARM, ok := os.LookupEnv("SKIP_ARM")
	if ok && strings.EqualFold(skipARM, "true") {
		return nil, nil
	}

	cli, err := ucpapi.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(ucpconn))
	if err != nil {
		return nil, err
	}

	option := &armauth.Options{
		SecretProvider:      sprovider.NewSecretProvider(cfg.SecretProvider),
		UCPCredentialClient: cli,
	}

	arm, err := armauth.NewArmConfig(option)
	if err != nil {
		return nil, fmt.Errorf("failed to build ARM config: %w", err)
	}

	return arm, nil
}

func NewHostOptionsFromEnvironment(configPath string) (HostOptions, error) {
	conf, err := loadConfig(configPath)
	if err != nil {
		return HostOptions{}, err
	}

	k8s, err := getKubernetes()
	if err != nil {
		return HostOptions{}, err
	}

	ucp, err := getUCPConnection(conf, k8s)
	if err != nil {
		return HostOptions{}, err
	}

	arm, err := getArmConfig(conf, ucp)
	if err != nil {
		return HostOptions{}, err
	}

	return HostOptions{
		Config:        conf,
		K8sConfig:     k8s,
		Arm:           arm,
		UCPConnection: ucp,
	}, nil
}

func loadConfig(configPath string) (*ProviderConfig, error) {
	buf, err := os.ReadFile(configPath)
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
	return ctx.Value(v1.HostingConfigContextKey).(*ProviderConfig)
}

// WithContext injects ProviderConfig into the given http context.
func WithContext(ctx context.Context, cfg *ProviderConfig) context.Context {
	return context.WithValue(ctx, v1.HostingConfigContextKey, cfg)
}

func getKubernetes() (*rest.Config, error) {
	cfg, err := kube.GetConfig()
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

func getUCPConnection(config *ProviderConfig, k8sConfig *rest.Config) (sdk.Connection, error) {
	if config.UCP.Kind == UCPConnectionKindDirect {
		if config.UCP.Direct == nil || config.UCP.Direct.Endpoint == "" {
			return nil, errors.New("the property .ucp.direct.endpoint is required when using a direct connection")
		}

		return sdk.NewDirectConnection(config.UCP.Direct.Endpoint)
	}

	return sdk.NewKubernetesConnectionFromConfig(k8sConfig)
}
