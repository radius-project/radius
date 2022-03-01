package bindings

import (
	"context"
	"log"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

func ServiceBusBinding(envParams map[string]string) BindingStatus {
	queueName := envParams["QUEUE"]
	if queueName == "" {
		log.Fatal("QUEUE is required")
		return BindingStatus{false, "QUEUE is required"}
	}
	connStr := envParams["CONNECTIONSTRING"]
	if connStr == "" {
		log.Fatal("CONNECTIONSTRING is required")
		return BindingStatus{false, "CONNECTIONSTRING is required"}
	}
	//Client to connect to the service bus wirh the CONNECTIONSTRING as namespace
	// erroe out if the connection fails
	ns, err := servicebus.NewNamespace(servicebus.NamespaceWithConnectionString(connStr))
	if err != nil {
		log.Fatal("failed to create service bus connection - ", err.Error())
		return BindingStatus{false, "Connection failed"}
	}

	//create a context to terminate the servicebus queue manager
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sourceQueue, err := ns.NewQueue(queueName)
	if err != nil {
		log.Fatalf("fetching queue %s failed with error - %s", queueName, err.Error())
		return BindingStatus{false, "Queue - " + queueName + " not found"}
	}
	defer func() {
		_ = sourceQueue.Close(ctx)
	}()
	if err := sourceQueue.Send(ctx, servicebus.NewMessageFromString("hello, world!")); err != nil {
		log.Fatal("failed to send message - ", err.Error())
		return BindingStatus{false, "Message sent - failed"}
	}
	return BindingStatus{true, "message sent"}
}
