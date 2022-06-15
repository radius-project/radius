package bindings

import (
	"context"
	"fmt"
	"log"
	"time"

	redis "github.com/go-redis/redis/v8"
)

func RedisBinding(envParams map[string]string) BindingStatus {
	// session from your local session pool.
	var err error
	redisHost := envParams["HOST"] + ":" + envParams["PORT"]
	if redisHost == ":" {
		log.Println("redis HOST and PORT are required")
		return BindingStatus{false, "Redis HOST and PORT are required"}
	}
	redisPassword := envParams["PASSWORD"]
	op := &redis.Options{Addr: redisHost, Password: redisPassword, WriteTimeout: 5 * time.Second}
	client := redis.NewClient(op)

	ctx := context.Background()
	err = client.Ping(ctx).Err()
	if err != nil {
		log.Println(fmt.Sprintf("failed to connect with redis instance at %s - %v", redisHost, err.Error()))
		return BindingStatus{false, "not connected"}
	}
	return BindingStatus{true, "connected"}
}
