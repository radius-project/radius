package bindings

import (
	"context"
	"crypto/tls"
	"log"
	"time"

	redis "github.com/go-redis/redis/v8"
)

// # Function Explanation
// 
// RedisBinding checks if a connection can be established with a Redis instance using the provided environment parameters 
// and returns a BindingStatus indicating if the connection was successful.
func RedisBinding(envParams map[string]string) BindingStatus {
	// session from your local session pool.
	var err error
	redisHost := envParams["HOST"] + ":" + envParams["PORT"]
	if redisHost == ":" {
		log.Println("redis HOST and PORT are required")
		return BindingStatus{false, "Redis HOST and PORT are required"}
	}
	redisPassword := envParams["PASSWORD"]
	op := &redis.Options{
		Addr:         redisHost,
		Password:     redisPassword,
		WriteTimeout: 5 * time.Second,
	}

	// Enable TLS if the port is 6380.
	if envParams["PORT"] == "6380" {
		op.TLSConfig = &tls.Config{}
	}

	client := redis.NewClient(op)

	ctx := context.Background()
	err = client.Ping(ctx).Err()
	if err != nil {
		log.Printf("failed to connect with redis instance at %s - %v\n", redisHost, err.Error())
		return BindingStatus{false, "not connected"}
	}
	return BindingStatus{true, "connected"}
}
