package bindings

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// ServiceBusBinding checks if the environment variables are set, creates a service bus connection, sends and receives a
// message and returns a BindingStatus.
func ServiceBusBinding(envParams map[string]string) BindingStatus {
	queueName := envParams["QUEUE"]
	if queueName == "" {
		log.Println("QUEUE is required")
		return BindingStatus{false, "QUEUE is required"}
	}

	namespace := envParams["CONNECTIONSTRING"]
	if namespace == "" {
		log.Println("CONNECTIONSTRING is required")
		return BindingStatus{false, "CONNECTIONSTRING is required"}
	}

	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Printf("Failed to create credential. Error: %s", err.Error())
		return BindingStatus{false, "Credential creation failed"}
	}

	// Client to connect to the service bus with the CONNECTIONSTRING as namespace
	// errors out if the connection fails

	client, err := azservicebus.NewClient(namespace, credential, nil)
	if err != nil {
		log.Printf("Failed to create service bus connection with namespace: %s\nError: %s\n", namespace, err.Error())
		return BindingStatus{false, "Connection failed"}
	}

	// create a context to terminate the servicebus queue manager
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	message := "hello, world!"
	err = SendMessage(ctx, message, queueName, client)
	if err != nil {
		log.Printf("Failed to send message: %s\nError: %s\n", message, err.Error())
		return BindingStatus{false, "Message couldn't be sent"}
	}

	messages, err := GetMessage(ctx, 1, queueName, client)
	if err != nil {
		log.Printf("Failed to get message from queue: %s\nError: %s\n", queueName, err.Error())
		return BindingStatus{false, "Message couldn't be received"}
	}

	if len(messages) != 1 {
		log.Println("Received message count is not 1")
		return BindingStatus{false, "Received message count is not 1"}
	}

	if messages[0] != message {
		log.Println("Message received is not the same as the message sent")
		return BindingStatus{false, "Message received is not the same as the message sent"}
	}

	return BindingStatus{true, "Message sent and received successfully"}
}

// SendMessage creates a new sender, sends a message and returns an error if any of the operations fail.
func SendMessage(ctx context.Context, message string, queueName string, client *azservicebus.Client) error {
	sender, err := client.NewSender(queueName, nil)
	if err != nil {
		log.Printf("Failed to create a new sender. Error: %s", err.Error())
		return err
	}
	defer sender.Close(ctx)

	sbMessage := &azservicebus.Message{
		Body: []byte(message),
	}
	err = sender.SendMessage(ctx, sbMessage, nil)
	if err != nil {
		log.Printf("Failed to send the message: %s\nError: %s", message, err.Error())
		return err
	}

	return nil
}

// // GetMessage creates a new receiver for a given queue, receives messages from the queue, logs the messages and returns
// them as a slice of strings. It returns an error if any of the operations fail.
func GetMessage(ctx context.Context, count int, queueName string, client *azservicebus.Client) ([]string, error) {
	var result []string

	receiver, err := client.NewReceiverForQueue(queueName, nil)
	if err != nil {
		log.Printf("Failed to create a new receiver. Error: %s", err.Error())
		return result, err
	}
	defer receiver.Close(ctx)

	messages, err := receiver.ReceiveMessages(ctx, count, nil)
	if err != nil {
		log.Printf("Failed to receive messages. Error: %s", err.Error())
		return result, err
	}

	for _, message := range messages {
		body := message.Body
		log.Printf("%s\n", string(body))

		result = append(result, fmt.Sprintf("%s", body))

		err = receiver.CompleteMessage(ctx, message, nil)
		if err != nil {
			log.Printf("failed to complete the message with ID: %s\nError: %s", message.MessageID, err.Error())
			return result, err
		}
	}

	return result, nil
}
