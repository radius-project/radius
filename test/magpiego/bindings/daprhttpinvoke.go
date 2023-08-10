package bindings

import (
	"context"
	"fmt"
	"log"
	"os"

	dapr "github.com/dapr/go-sdk/client"
)

// # Function Explanation
//
// DaprHttpRouteBinding checks if the environment parameter "APPID" is present and if so, creates a Dapr client and invokes
// a method on it, returning a BindingStatus object with a boolean and a message. If an error occurs, the BindingStatus
// object will contain false and an error message.
//
// requires both the value to be set as env variables
// - CONNECTION_DAPRHTTPROUTE_APPID
// - DAPR_GRPC_PORT
func DaprHttpRouteBinding(envParams map[string]string) BindingStatus {
	appID := envParams["APPID"]
	if appID == "" {
		log.Println("APPID is required")
		return BindingStatus{false, "APPID is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("Error creating dapr client", err)
		return BindingStatus{false, fmt.Sprintf("Error creating dapr client - %s", err.Error())}
	}
	defer client.Close()
	ctx := context.Background()
	//Using Dapr SDK to invoke a method
	result, err := client.InvokeMethod(ctx, appID, "/backend", "get")
	if err != nil {
		log.Println("Error invoking dapr InvokeMethod - ", err)
		return BindingStatus{false, fmt.Sprintf("Error invoking dapr InvokeMethod  - %s", err.Error())}
	}
	log.Printf("successfully invoked dapr InvokeMethod. Response: %s", string(result))
	return BindingStatus{true, "invoked dapr InvokeMethod"}
}
