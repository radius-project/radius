package main

import (
	"fmt"
	"os"
	dapr "github.com/dapr/go-sdk/client"
)

func main() {
	a := App{}

	client, err := dapr.NewClient()
	if err != nil {
		panic(err)
	}
	a.Initialize(
		client,
	)

	port, ok := os.LookupEnv("PORT");

	if !ok {
		port = "8050"
	}

	binding := fmt.Sprintf(":%s", port)

	a.Run(binding)
	defer client.Close()
}
