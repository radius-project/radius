package bindings

import (
	"context"
	"log"

	"os"

	dapr "github.com/dapr/go-sdk/client"
)

func DaprPubSubBinding(envParams map[string]string) BindingStatus {
	//From https://docs.dapr.io/developing-applications/building-blocks/pubsub/howto-publish-subscribe/
	pubSubName := envParams["NAME"]
	if pubSubName == "" {
		log.Println("pub sub NAME is required")
		return BindingStatus{false, "NAME is required"}
	}
	topic := envParams["TOPIC"]
	if topic == "" {
		log.Println("TOPIC sub NAME is required")
		return BindingStatus{false, "TOPIC is required"}
	}
	client, err := dapr.NewClientWithPort(os.Getenv("DAPR_GRPC_PORT"))
	if err != nil {
		log.Println("failed to create Dapr client - ", err.Error())
		return BindingStatus{false, "failed to publish Dapr"}
	}
	ctx := context.Background()
	//Using Dapr SDK to publish a topic
	if err := client.PublishEvent(ctx, pubSubName, topic, []byte("hello, world!")); err != nil {
		log.Println("failed to publish Dapr event - ", pubSubName, " error - ", err.Error())
		return BindingStatus{false, "failed to publish Dapr"}
	}
	defer client.Close()
	return BindingStatus{true, "message sent"}
}
