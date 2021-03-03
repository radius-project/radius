// package config manages loading configuration from environment and command-line params
package config

import (
	"fmt"
	"os"

	"github.com/marstr/randname"
)

var (
	// these are our *global* config settings, to be shared by all packages.
	// each has corresponding public accessors below.
	// if anything requires a `Set` accessor, that indicates it perhaps
	// shouldn't be set here, because mutable vars shouldn't be global.
	clientID               string
	clientSecret           string
	tenantID               string
	subscriptionID         string
	locationDefault        string
	cloudName              string = "AzurePublicCloud"
	baseGroupName          string
	userAgent              string
)

// ClientID is the OAuth client ID.
func ClientID() string {
	return clientID
}

// ClientSecret is the OAuth client secret.
func ClientSecret() string {
	return clientSecret
}

// TenantID is the AAD tenant to which this client belongs.
func TenantID() string {
	return tenantID
}

// SubscriptionID is a target subscription for Azure resources.
func SubscriptionID() string {
	return subscriptionID
}

// DefaultLocation() returns the default location wherein to create new resources.
// Some resource types are not available in all locations so another location might need
// to be chosen.
func DefaultLocation() string {
	return locationDefault
}

// BaseGroupName() returns a prefix for new groups.
func BaseGroupName() string {
	return baseGroupName
}

// UserAgent() specifies a string to append to the agent identifier.
func UserAgent() string {
	if len(userAgent) > 0 {
		return userAgent
	}
	return "radius-test"
}

// Read test configuration from environment variables
func Read()
{
	clientID = os.Getenv(RADIUS_TEST_CLIENT_ID)
	clientSecret = os.Getenv(RADIUS_TEST_CLIENT_SECRET)
	tenantID = os.Getenv(RADIUS_TEST_TENANT_ID)
	subscriptionID = os.Getenv(RADIUS_TEST_SUBSCRIPTION_ID)
	locationDefault = os.Getenv(RADIUS_TEST_DEFAULT_LOCATION)
	baseGroupName = os.Getenv(RADIUS_TEST_BASE_GROUP_NAME)
}