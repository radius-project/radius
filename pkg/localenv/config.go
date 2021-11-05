package localenv

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetRESTConfig(kubeConfigPath string) (*rest.Config, error) {
	rawconfig, err := clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("unable to get Kubernetes client config: %w", err)
	}

	context := rawconfig.Contexts[rawconfig.CurrentContext]
	if context == nil {
		return nil, fmt.Errorf("kubernetes context '%s' could not be found", rawconfig.CurrentContext)
	}

	clientconfig := clientcmd.NewNonInteractiveClientConfig(*rawconfig, rawconfig.CurrentContext, nil, nil)
	merged, err := clientconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return merged, nil
}
