package bindings

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// FROM https://www.digitalocean.com/community/tutorials/how-to-use-go-with-mongodb-using-the-mongodb-go-driver
var ctx = context.TODO()

// MongoBinding checks if the CONNECTIONSTRING environment parameter is present and if so, attempts to connect to a MongoDB
//
//	instance using the provided URI, returning a BindingStatus indicating success or failure.
func MongoBinding(envParams map[string]string) BindingStatus {
	uri := envParams["CONNECTIONSTRING"]
	if uri == "" {
		log.Println("CONNECTIONSTRING is required")
		return BindingStatus{false, "CONNECTIONSTRING is required"}
	}
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Println("mongodb connection failed", err.Error())
		return BindingStatus{false, "mongodb connection failed"}
	}
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Println("mongodb connection failed", err.Error())
		return BindingStatus{false, "mongodb connection failed"}
	}

	return BindingStatus{true, "connected"}
}
