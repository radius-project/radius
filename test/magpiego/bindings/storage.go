package bindings

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// requires the following environment variables:
// - CONNECTION_STORAGE_NAME
func StorageBinding(envParams map[string]string) BindingStatus {
	storageAccountName := envParams["NAME"]
	if storageAccountName == "" {
		log.Println("Azure Storage Account Name is required")
		return BindingStatus{false, "AZURE_STORAGE_ACCOUNT_NAME is required"}
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Println("Failed to create credential")
		return BindingStatus{false, "Failed to create credential"}
	}

	client, err := azblob.NewClient(storageAccountName, cred, nil)
	if err != nil {
		log.Println("Failed to create client")
		return BindingStatus{false, "Failed to create client"}
	}

	containerName := fmt.Sprintf("magpiego-%s", randomString())

	resp, err := client.CreateContainer(context.TODO(), containerName, &azblob.CreateContainerOptions{
		Metadata: map[string]string{"hello": "world"},
	})
	if err != nil {
		log.Println("Failed to create container")
		return BindingStatus{false, "Failed to create container"}
	}

	log.Printf("Successfully created a blob container %q. Response: %s", containerName, string(*resp.RequestID))
	return BindingStatus{true, "Created blob container"}
}

func randomString() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return strconv.Itoa(r.Int())
}
