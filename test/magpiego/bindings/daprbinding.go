package bindings

import (
	"context"
	"log"

	dapr "github.com/dapr/go-sdk/client"
	"os"
)

// DaprBindingBinding checks if the environment parameter COMPONENTNAME is set and if so, creates a Dapr client and
// retrieves a sample key from the Dapr bound store
//
// Use this with a values like:
// - CONNECTION_DAPRBINDING_COMPONENTNAME
// - DAPR_GRPC_PORT
func DaprBindingBinding(envParams map[string]string) BindingStatus {
	// From https://docs.dapr.io/getting-started/quickstarts/configuration-quickstart/
	componentName := envParams["COMPONENTNAME"]
	if componentName == "" {
		log.Println("COMPONENTNAME is required")
		return BindingStatus{false, "COMPONENTNAME is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "Failed to retrieve value from binding"}
	}
	ctx := context.Background()
	_, err = client.InvokeBinding(ctx, &dapr.InvokeBindingRequest{
		Name:      componentName,
		Operation: "get",
		Metadata: map[string]string{
			"key": "test-binding",
		},
	})
	if err != nil {
		log.Println("failed to get Dapr binding item - ", componentName, " error - ", err.Error())
	}
	defer client.Close()
	return BindingStatus{true, "Binding value retrieved"}
}
