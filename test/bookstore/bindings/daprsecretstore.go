package bindings

import (
	"context"
	"log"
	"os"

	dapr "github.com/dapr/go-sdk/client"
)

func DaprSecretStoreBinding(envParams map[string]string) BindingStatus {
	secretName := envParams["SECRETSTORENAME"]
	if secretName == "" {
		log.Println("SECRETSTORENAME is required")
		return BindingStatus{false, "SECRETSTORENAME is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "Error creating Dapr client"}
	}
	defer client.Close()
	ctx := context.Background()
	if _, err := client.GetSecret(ctx, secretName, "SOME_SECRET", nil); err != nil {
		log.Println("failed to get the secret from Dapr secret store - ", secretName, " error - ", err.Error())
		return BindingStatus{false, "failed to get secret from Dapr"}
	}
	return BindingStatus{true, "secrets accessed"}
}
