package bindings

import (
	"context"
	"log"

	dapr "github.com/dapr/go-sdk/client"
)

func DaprPubSubBinding(envParams map[string]string) BindingStatus {
	//From https://docs.dapr.io/developing-applications/building-blocks/pubsub/howto-publish-subscribe/
	pubSubName := envParams["NAME"]
	if pubSubName == "" {
		log.Fatal("pub sub NAME is required")
		return BindingStatus{false, "NAME is required"}
	}
	topic := envParams["TOPIC"]
	if topic == "" {
		log.Fatal("TOPIC sub NAME is required")
		return BindingStatus{false, "TOPIC is required"}
	}
	client, err := dapr.NewClient()
	if err != nil {
		log.Fatal("failed to create Dapr client - ", err.Error())
	}
	defer client.Close()
	ctx := context.Background()
	//Using Dapr SDK to publish a topic
	if err := client.PublishEvent(ctx, pubSubName, topic, []byte("hello, world!")); err != nil {
		log.Fatal("failed to publish Dapr event - ", pubSubName, " error - ", err.Error())
		return BindingStatus{false, "failed to publish Dapr"}
	}
	return BindingStatus{true, "message sent"}
}
