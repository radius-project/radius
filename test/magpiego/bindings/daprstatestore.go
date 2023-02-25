package bindings

import (
	"context"
	"log"
	"os"

	dapr "github.com/dapr/go-sdk/client"
)

func DaprStateStoreBinding(envParams map[string]string) BindingStatus {
	// From https://docs.dapr.io/developing-applications/sdks/go/go-client/
	stateStoreName := envParams["COMPONENTNAME"]
	if stateStoreName == "" {
		log.Println("COMPONENTNAME is required")
		return BindingStatus{false, "COMPONENTNAME is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "Error creating Dapr client"}
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.SaveState(ctx, stateStoreName, "key", []byte("value"), map[string]string{}); err != nil {
		log.Println("failed to save to the Dapr state store - ", stateStoreName, " error - ", err.Error())
		return BindingStatus{false, "failed to save to the Dapr state store"}
	}
	log.Println("successfully saved to the state store - ", stateStoreName)
	return BindingStatus{true, "message sent"}
}
