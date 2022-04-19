// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

// ConnectionOptions represents connection info to connect CosmosDB
type ConnectionOptions struct {
	// Url represents the url of cosmosdb endpoint.
	Url string
	// DatabaseName represents the database name to connect.
	DatabaseName string
	// CollectionName represents the collection name in DataBaseName
	CollectionName string
	// MaxQueryItemCount represents the maximum number of items for query.
	MaxQueryItemCount int

	// KeyAuth represents an authentication option using master key.
	KeyAuth *CosmosDBKeyAuthOptions
	// AzureAdAuth reprsents an authentication option using Azure AD.
	AzureADAuth *AzureADAuthOptions
}

// CosmosDBKeyAuthOptions represents authentication options using master key.
type CosmosDBKeyAuthOptions struct {
	// MasterKey is the key string for CosmosDB connection.
	MasterKey string
}

// AzureADAuthOptions represents authentication options using Azure AD.
type AzureADAuthOptions struct {
	// Endpoint is Azure AD Login Endpoint.
	Endpoint string
	// Audience is an target audience.
	Audience string

	// UseMSI is the flag to use managed identity.
	UseMSI bool
	// TenantID is an tenant id of Azure.
	TenantID string
	// ClientID is the client id of AAD identity.
	ClientID string
	// ClientSecret is the client secret of AAD identity. (optional)
	ClientSecret string
}
