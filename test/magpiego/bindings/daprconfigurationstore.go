package bindings

import (
	"context"
	"log"

	dapr "github.com/dapr/go-sdk/client"
	"os"
)

// DaprConfigurationStoreBinding checks if the environment parameter COMPONENTNAME is set and if so, creates a Dapr client and
// retrieves a configuration item from the Dapr configuration store.
//
// Use this with a values like:
// - CONNECTION_DAPRCONFIGURATIONSTORE_COMPONENTNAME
// - DAPR_GRPC_PORT
func DaprConfigurationStoreBinding(envParams map[string]string) BindingStatus {
	// From https://docs.dapr.io/getting-started/quickstarts/configuration-quickstart/
	componentName := envParams["COMPONENTNAME"]
	if componentName == "" {
		log.Println("COMPONENTNAME is required")
		return BindingStatus{false, "COMPONENTNAME is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "Failed to retrieve config item"}
	}
	ctx := context.Background()
	_, err = client.GetConfigurationItem(ctx, componentName, "myconfig")
	if err != nil {
		log.Println("failed to get Dapr configuration item - ", componentName, " error - ", err.Error())
	}
	defer client.Close()
	return BindingStatus{true, "Config item retrieved"}
}
