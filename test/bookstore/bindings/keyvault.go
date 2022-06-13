package bindings

import (
	"context"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

func KeyVaultBinding(envParams map[string]string) BindingStatus {
	// FROM: https://docs.microsoft.com/en-us/azure/key-vault/secrets/quick-create-go
	keyVaultUrl := envParams["URI"]
	if keyVaultUrl == "" {
		log.Println("URI is required")
		return BindingStatus{false, "URI is required"}
	}
	// Create a credential using the NewDefaultAzureCredential type.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Println("failed to obtain a credential - ", err.Error())
		return BindingStatus{false, "failed to obtain a credential"}
	}

	// Establish a connection to the Key Vault client
	client, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		log.Println("failed to connect to keyVault client - ", err.Error())
		return BindingStatus{false, "failed to connect to keyVault client"}
	}
	_, err = client.SetSecret(context.TODO(), "TestSecret", "testValue", nil)
	if err != nil {
		log.Println(fmt.Sprintf("failed to create a secret: %v", err))
		return BindingStatus{false, "failed to create a secret"}
	}
	_, err = client.GetSecret(context.TODO(), "TestSecret", nil)
	if err != nil {
		log.Fatalf("failed to get the secret: %v", err)
		return BindingStatus{false, "failed to get the secret"}
	}
	return BindingStatus{true, "secrets accessed"}
}
