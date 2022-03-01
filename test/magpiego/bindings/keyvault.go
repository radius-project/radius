package bindings

import (
	"context"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

func KeyVaultBinding(envParams map[string]string) BindingStatus {
	//FROM: https://docs.microsoft.com/en-us/azure/key-vault/secrets/quick-create-go
	keyVaultUrl := envParams["URI"]
	if keyVaultUrl == "" {
		log.Fatal("URI is required")
		return BindingStatus{true, "URI is required"}
	}
	//Create a credential using the NewDefaultAzureCredential type.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatal("failed to obtain a credential - ", err.Error())
		return BindingStatus{true, "failed to obtain a credential"}
	}

	//Establish a connection to the Key Vault client
	client, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		log.Fatal("failed to connect to keyVault client - ", err.Error())
		return BindingStatus{true, "failed to connect to keyVault client"}
	}
	pages := client.ListSecrets(nil)
	for pages.NextPage(context.TODO()) {
		return BindingStatus{true, "secrets accessed"}
	}
	return BindingStatus{false, "secrets not accessed"}
}
