package bindings

import (
	"context"
	"log"

	"os"

	dapr "github.com/dapr/go-sdk/client"
)

// # Function Explanation
//
// DaprPubSubBinding checks if the environment parameter COMPONENTNAME is set and if so, creates a Dapr client and
// publishes an event to a topic.
//
// Use this with a values like:
// - CONNECTION_DAPRPUBSUB_COMPONENTNAME
// - DAPR_GRPC_PORT
func DaprPubSubBinding(envParams map[string]string) BindingStatus {
	// From https://docs.dapr.io/developing-applications/building-blocks/pubsub/howto-publish-subscribe/
	componentName := envParams["COMPONENTNAME"]
	if componentName == "" {
		log.Println("COMPONENTNAME is required")
		return BindingStatus{false, "COMPONENTNAME is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "failed to publish Dapr"}
	}
	ctx := context.Background()
	// Using Dapr SDK to publish a topic
	if err := client.PublishEvent(ctx, componentName, "testTopic", []byte("hello, world!")); err != nil {
		log.Println("failed to publish Dapr event - ", componentName, " error - ", err.Error())
		return BindingStatus{false, "failed to publish Dapr"}
	}
	defer client.Close()
	return BindingStatus{true, "message sent"}
}
