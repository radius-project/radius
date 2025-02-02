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
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/azure/armauth"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	sprovider "github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/config"
	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"

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

	provider, err := sdk_cred.NewAzureCredentialProvider(sprovider.NewSecretProvider(cfg.SecretProvider), ucpconn, &aztoken.AnonymousCredential{})
	if err != nil {
		return nil, err
	}

	arm, err := armauth.NewArmConfig(&armauth.Options{
		CredentialProvider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build ARM config: %w", err)
	}

	return arm, nil
}

// NewHostOptionsFromEnvironment loads configuration from a given path, retrieves Kubernetes and ARM configurations, and
// returns a HostOptions object containing the configurations, or an error if any of the operations fail.
func NewHostOptionsFromEnvironment(configPath string) (HostOptions, error) {
	conf, err := loadConfig(configPath)
	if err != nil {
		return HostOptions{}, err
	}

	k8s, err := getKubernetes()
	if err != nil {
		return HostOptions{}, err
	}

	ucp_conn, err := config.NewConnectionFromUCPConfig(&conf.UCP, k8s)
	if err != nil {
		return HostOptions{}, err
	}

	arm, err := getArmConfig(conf, ucp_conn)
	if err != nil {
		return HostOptions{}, err
	}

	return HostOptions{
		Config:        conf,
		K8sConfig:     k8s,
		Arm:           arm,
		UCPConnection: ucp_conn,
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
	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		// TODO: Allow to use custom context via configuration. - https://github.com/radius-project/radius/issues/5433
		ContextName: "",
		QPS:         kubeutil.DefaultServerQPS,
		Burst:       kubeutil.DefaultServerBurst,
	})
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
