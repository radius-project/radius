package bindings

import (
	"context"
	"log"
	"os"

	dapr "github.com/dapr/go-sdk/client"
)

// DaprSecretStoreBinding checks if the required environment parameters are present and if so, creates a Dapr client and
// attempts to get the secret from the Dapr secret store.
func DaprSecretStoreBinding(envParams map[string]string) BindingStatus {
	secretName := envParams["COMPONENTNAME"]
	if secretName == "" {
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
	if _, err := client.GetSecret(ctx, secretName, "mysecret", nil); err != nil {
		log.Println("failed to get the secret from Dapr secret store - ", secretName, " error - ", err.Error())
		return BindingStatus{false, "failed to get secret from Dapr"}
	}
	log.Println("successfully got the secret from the secret store - ", secretName)
	return BindingStatus{true, "secrets accessed"}
}
