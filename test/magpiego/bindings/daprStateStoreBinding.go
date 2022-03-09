package bindings

import (
	"context"
	"log"

	dapr "github.com/dapr/go-sdk/client"
)

func DaprStateStoreBinding(envParams map[string]string) BindingStatus {
	// From https://docs.dapr.io/developing-applications/sdks/go/go-client/
	stateStoreName := envParams["STATESTORENAME"]
	if stateStoreName == "" {
		log.Println("STATESTORENAME is required")
		return BindingStatus{false, "STATESTORENAME is required"}
	}
	client, err := dapr.NewClient()
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "Error creating Dapr client"}
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.SaveState(ctx, stateStoreName, "key", []byte("value")); err != nil {
		log.Println("failed to save to the Dapr state store - ", stateStoreName, " error - ", err.Error())
		return BindingStatus{false, "failed to save to the Dapr state store"}
	}
	return BindingStatus{true, "message sent"}
}
