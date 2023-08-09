package bindings

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

// # Function Explanation
//
// StorageBinding checks for required environment variables, creates a credential and a client, creates a container and
// marks it for deletion.
//
// requires the following environment variables:
// - CONNECTION_STORAGE_ACCOUNT
func StorageBinding(envParams map[string]string) BindingStatus {
	storageAccountName := envParams["ACCOUNTNAME"]
	if storageAccountName == "" {
		log.Println("CONNECTION_STORAGE_ACCOUNTNAME is required")
		return BindingStatus{false, "CONNECTION_STORAGE_ACCOUNTNAME is required"}
	}

	url := "https://" + storageAccountName + ".blob.core.windows.net/"

	clientID := os.Getenv("AZURE_CLIENT_ID")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	tokenFilePath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	authorityHost := os.Getenv("AZURE_AUTHORITY_HOST")

	if clientID == "" {
		log.Println("AZURE_CLIENT_ID is required")
		return BindingStatus{false, "AZURE_CLIENT_ID is required"}
	}
	if tenantID == "" {
		log.Println("AZURE_TENANT_ID is required")
		return BindingStatus{false, "AZURE_TENANT_ID is required"}
	}
	if tokenFilePath == "" {
		log.Println("AZURE_FEDERATED_TOKEN_FILE is required")
		return BindingStatus{false, "AZURE_FEDERATED_TOKEN_FILE is required"}
	}
	if authorityHost == "" {
		log.Println("AZURE_AUTHORITY_HOST is required")
		return BindingStatus{false, "AZURE_AUTHORITY_HOST is required"}
	}

	cred, err := newClientAssertionCredential(tenantID, clientID, authorityHost, tokenFilePath, nil)
	if err != nil {
		log.Println("Failed to create credential")
		return BindingStatus{false, "Failed to create credential"}
	}

	client, err := azblob.NewClient(url, cred, nil)
	if err != nil {
		log.Println("Failed to create client")
		return BindingStatus{false, "Failed to create client"}
	}

	containerName := fmt.Sprintf("magpiego-%s", randomString())
	log.Println("Container Name: " + containerName)

	// Create a container
	resp, err := client.CreateContainer(context.TODO(), containerName, nil)
	if err != nil {
		log.Printf("Failed to create container: %s\n", err.Error())
		return BindingStatus{false, "Failed to create container"}
	}
	log.Printf("Successfully created a blob container %q. Response: %s\n", containerName, string(*resp.RequestID))

	// Delete the container
	delResp, err := client.DeleteContainer(context.TODO(), containerName, nil)
	if err != nil {
		log.Printf("Failed to mark container for deletion: %s\n", err.Error())
		return BindingStatus{false, "Failed to mark container for deletion"}
	}
	log.Printf("Successfully marked the container for deletion %q. Response: %s\n", containerName, string(*delResp.RequestID))

	return BindingStatus{true, "Created a container and marked it for deletion successfully"}
}

func randomString() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return strconv.Itoa(r.Int())
}

// clientAssertionCredential authenticates an application with assertions provided by a callback function.
type clientAssertionCredential struct {
	assertion, file string
	client          confidential.Client
	lastRead        time.Time
}

// clientAssertionCredentialOptions contains optional parameters for ClientAssertionCredential.
type clientAssertionCredentialOptions struct {
	azcore.ClientOptions
}

// newClientAssertionCredential constructs a clientAssertionCredential. Pass nil for options to accept defaults.
func newClientAssertionCredential(tenantID, clientID, authorityHost, file string, options *clientAssertionCredentialOptions) (*clientAssertionCredential, error) {
	c := &clientAssertionCredential{file: file}

	cred := confidential.NewCredFromAssertionCallback(
		func(ctx context.Context, _ confidential.AssertionRequestOptions) (string, error) {
			return c.getAssertion(ctx)
		},
	)

	authority := fmt.Sprintf("%s%s/oauth2/token", authorityHost, tenantID)
	client, err := confidential.New(authority, clientID, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create confidential client: %w", err)
	}
	c.client = client

	return c, nil
}

// GetToken implements the TokenCredential interface
//
// # Function Explanation
//
// // clientAssertionCredential.GetToken retrieves an access token from the confidential client and returns it as an
// azcore.AccessToken. If an error occurs, it is returned.
func (c *clientAssertionCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	// get the token from the confidential client
	token, err := c.client.AcquireTokenByCredential(ctx, opts.Scopes)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	return azcore.AccessToken{
		Token:     token.AccessToken,
		ExpiresOn: token.ExpiresOn,
	}, nil
}

// getAssertion reads the assertion from the file and returns it
// if the file has not been read in the last 5 minutes
func (c *clientAssertionCredential) getAssertion(context.Context) (string, error) {
	if now := time.Now(); c.lastRead.Add(5 * time.Minute).Before(now) {
		content, err := os.ReadFile(c.file)
		if err != nil {
			return "", err
		}
		c.assertion = string(content)
		c.lastRead = now
	}
	return c.assertion, nil
}
