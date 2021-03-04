// package config manages loading configuration from environment and command-line params
package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/marstr/randname"
)

var (
	// these are our *global* config settings, to be shared by all packages.
	// each has corresponding public accessors below.
	// if anything requires a `Set` accessor, that indicates it perhaps
	// shouldn't be set here, because mutable vars shouldn't be global.
	clientID        string
	clientSecret    string
	tenantID        string
	subscriptionID  string
	locationDefault string
	cloudName       string = "AzurePublicCloud"
	useDeviceFlow   bool
	baseGroupName   string
	userAgent       string
	environment     *azure.Environment
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

// UseDeviceFlow() specifies if interactive auth should be used. Interactive
// auth uses the OAuth Device Flow grant type.
func UseDeviceFlow() bool {
	return useDeviceFlow
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

// Environment() returns an `azure.Environment{...}` for the current cloud.
func Environment() *azure.Environment {
	if environment != nil {
		return environment
	}
	env, err := azure.EnvironmentFromName(cloudName)
	if err != nil {
		// TODO: move to initialization of var
		panic(fmt.Sprintf(
			"invalid cloud name '%s' specified, cannot continue\n", cloudName))
	}
	environment = &env
	return environment
}

// GenerateGroupName leverages BaseGroupName() to return a more detailed name,
// helping to avoid collisions.  It appends each of the `affixes` to
// BaseGroupName() separated by dashes, and adds a 5-character random string.
func GenerateGroupName(baseGroupName string, affixes ...string) string {
	// go1.10+
	// import strings
	// var b strings.Builder
	// b.WriteString(BaseGroupName())
	b := bytes.NewBufferString(baseGroupName)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

// Read test configuration from environment variables
func init() {
	clientID = os.Getenv("AZURE_CLIENT_ID")
	clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	tenantID = os.Getenv("AZURE_TENANT_ID")
	subscriptionID = os.Getenv("RADIUS_TEST_SUBSCRIPTION_ID")
	locationDefault = os.Getenv("RADIUS_TEST_DEFAULT_LOCATION")
	baseGroupName = os.Getenv("RADIUS_TEST_BASE_GROUP_NAME")
}
