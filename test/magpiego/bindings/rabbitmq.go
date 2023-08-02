package bindings

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func RabbitMQBinding(envParams map[string]string) BindingStatus {
	// From https://github.com/rabbitmq/rabbitmq-tutorials/blob/master/go/send.go
	connectionString := envParams["URI"]
	if connectionString == "" {
		log.Println("URI is required")
		return BindingStatus{false, "URI is required"}
	}

	queue := envParams["QUEUE"]
	if queue == "" {
		log.Println("QUEUE is required")
		return BindingStatus{false, "QUEUE is required"}
	}

	conn, err := amqp.Dial(connectionString)
	if err != nil {
		log.Println("Failed to connect to RabbitMQ - ", err.Error())
		return BindingStatus{false, "Failed to connect to RabbitMQ"}
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Println("Failed to open a channel - ", err.Error())
		return BindingStatus{false, "Failed to publish a message"}
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"hello", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		log.Println("Failed to declare a queue - ", err.Error())
		return BindingStatus{false, "Failed to declare a queue"}
	}
	msg := "Hello World!"
	err = ch.PublishWithContext(
		ctx,
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(msg),
		})
	if err != nil {
		log.Println("Failed to publish a message - ", err.Error())
		return BindingStatus{false, "Failed to publish a message"}
	}
	log.Println("sent ", msg)
	return BindingStatus{true, "message sent"}
}
