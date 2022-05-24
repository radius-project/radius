package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

type TestOptions struct {
	ConfigFilePath string
	K8sClient      *k8s.Clientset
	DynamicClient  dynamic.Interface
	Client         client.Client
}

func NewTestOptions(t *testing.T) TestOptions {
	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	k8sconfig, err := kubernetes.ReadKubeConfig()
	require.NoError(t, err, "failed to read k8s config")

	k8s, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes client")

	dynamicClient, err := kubernetes.CreateDynamicClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes dyamic client")

	client, err := kubernetes.CreateRuntimeClient(k8sconfig.CurrentContext, kubernetes.Scheme)
	require.NoError(t, err, "failed to create runtime client")

	return TestOptions{
		ConfigFilePath: config.ConfigFileUsed(),
		K8sClient:      k8s,
		Client:         client,
		DynamicClient:  dynamicClient,
	}
}
