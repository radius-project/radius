/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bicep

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	ucp_aws_frontend "github.com/radius-project/radius/pkg/ucp/frontend/aws"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// Test_Bicep_AddAWSProviderResourceIDs_ThroughUCP reads the ARNs of a recipe's AWS output resources through a real
// UCP with an in-memory Cloud Control. One resource is deployed by alias, so UCP has its record. The other is a
// resource the recipe references with existing: UCP has no record of it, so GET returns 404, and the driver reads it
// with POST /:get by the AWS identifier at the end of its ID.
func Test_Bicep_AddAWSProviderResourceIDs_ThroughUCP(t *testing.T) {
	const (
		bucketType = "AWS::S3::Bucket"
		prefix     = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.S3/Bucket/"
		aliasID    = prefix + "app-bucket"
		existingID = prefix + "existing-bucket"
	)

	cc := awsfake.NewCloudControl()
	host := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.AWSClients.CloudControl = cc
		options.AWSClients.CloudFormation = &awsfake.CloudFormation{Schemas: map[string]string{
			bucketType: `{"primaryIdentifier": ["/properties/BucketName"], "readOnlyProperties": ["/properties/Arn"], "createOnlyProperties": ["/properties/BucketName"]}`,
		}}
		options.Modules = []modules.Initializer{ucp_aws_frontend.NewModule(options)}
	}))

	cc.Put(bucketType, "deployed-bucket", map[string]any{"BucketName": "deployed-bucket", "Arn": "arn:aws:s3:::deployed-bucket"})
	cc.Put(bucketType, "existing-bucket", map[string]any{"BucketName": "existing-bucket", "Arn": "arn:aws:s3:::existing-bucket"})

	// Deploy app-bucket by alias. It takes over deployed-bucket and changes nothing, so UCP saves the record at once.
	body, err := json.Marshal(map[string]any{"properties": map[string]any{"BucketName": "deployed-bucket"}})
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, host.BaseURL()+aliasID+"?api-version=2023-10-01-preview", bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(sdkclients.AWSResourceOwnerHeader, "deploy;/planes/radius/local/resourceGroups/test;;")
	response, err := host.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	d := &bicepDriver{resourceClientFactory: func(*rest.Config) processors.ResourceClient {
		return processors.NewResourceClient(nil, host.Options().UCP, nil)
	}}
	output := &recipes.RecipeOutput{Resources: []string{aliasID, existingID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Equal(t, map[string]string{
		aliasID:    "arn:aws:s3:::deployed-bucket",
		existingID: "arn:aws:s3:::existing-bucket",
	}, output.ProviderResourceIDs)
}

// Test_Bicep_AddAWSProviderResourceIDs_ThroughUCP_ResourceGone checks that the driver never reads an unrelated resource
// by the alias. The alias app-bucket manages the bucket assets, which was deleted outside Radius, and an unrelated bucket
// is named app-bucket. GET returns 404 because AWS no longer has assets, not because UCP has no record, so the driver
// does not fall back to POST /:get, and the output resource gets no ARN.
func Test_Bicep_AddAWSProviderResourceIDs_ThroughUCP_ResourceGone(t *testing.T) {
	const (
		bucketType = "AWS::S3::Bucket"
		aliasID    = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.S3/Bucket/app-bucket"
	)

	cc := awsfake.NewCloudControl()
	host := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.AWSClients.CloudControl = cc
		options.AWSClients.CloudFormation = &awsfake.CloudFormation{Schemas: map[string]string{
			bucketType: `{"primaryIdentifier": ["/properties/BucketName"], "readOnlyProperties": ["/properties/Arn"], "createOnlyProperties": ["/properties/BucketName"]}`,
		}}
		options.Modules = []modules.Initializer{ucp_aws_frontend.NewModule(options)}
	}))

	cc.Put(bucketType, "assets", map[string]any{"BucketName": "assets", "Arn": "arn:aws:s3:::assets"})
	cc.Put(bucketType, "app-bucket", map[string]any{"BucketName": "app-bucket", "Arn": "arn:aws:s3:::app-bucket"})

	body, err := json.Marshal(map[string]any{"properties": map[string]any{"BucketName": "assets"}})
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, host.BaseURL()+aliasID+"?api-version=2023-10-01-preview", bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(sdkclients.AWSResourceOwnerHeader, "deploy;/planes/radius/local/resourceGroups/test;;")
	response, err := host.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	// assets is deleted outside Radius.
	cc.Remove(bucketType, "assets")

	d := &bicepDriver{resourceClientFactory: func(*rest.Config) processors.ResourceClient {
		return processors.NewResourceClient(nil, host.Options().UCP, nil)
	}}
	output := &recipes.RecipeOutput{Resources: []string{aliasID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Empty(t, output.ProviderResourceIDs, "the unrelated bucket named like the alias must not be read")
}
