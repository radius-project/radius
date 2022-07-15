// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/go-autorest/autorest"

	"github.com/project-radius/radius/test"
)

func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{
		TestOptions: test.NewTestOptions(t),

		// FYI we're not using this code anymore, so it's been hollowed out.

	}
}

type TestOptions struct {
	test.TestOptions
	ARMAuthorizer    autorest.Authorizer
	ARMConnection    *arm.Connection
	RadiusBaseURL    string
	RadiusConnection *arm.Connection
	RadiusSender     autorest.Sender
	Environment      *AzureCloudEnvironment
}

type AzureCloudEnvironment struct {
	RadiusEnvironment `mapstructure:",squash"`
	ClusterName       string `mapstructure:"clustername" validate:"required"`
	SubscriptionID    string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup     string `mapstructure:"resourcegroup" validate:"required"`
}

type RadiusEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication" yaml:",omitempty"`
	Scope              string `mapstructure:"scope,omitempty"`
	Id                 string `mapstructure:"id,omitempty"`

	// DEBUG STUFF:

	// RadiusRPLocalURL is an override for local debugging. This allows us us to run the controller + API Service outside the cluster.
	RadiusRPLocalURL         string `mapstructure:"radiusrplocalurl,omitempty"`
	DeploymentEngineLocalURL string `mapstructure:"deploymentenginelocalurl,omitempty"`
	UCPLocalURL              string `mapstructure:"ucplocalurl,omitempty"`
	UCPResourceGroupName     string `mapstructure:"ucpresourcegroupname,omitempty"`

	// Capture arbitrary other properties
	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}
