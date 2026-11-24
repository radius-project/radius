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

package redshift

import (
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const dataShareRegionName = "us-west-2"

func dataShareSchema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func dataShareWith(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

// dataShareMarkInternal puts the authorization or association of a data share Redshift made for itself under
// identifier, replacing any there, as nativetest.Suite.OtherProduct does.
func dataShareMarkInternal(typeName string) func(store *awsfake.CloudControl, identifier string) {
	return func(store *awsfake.CloudControl, identifier string) {
		arn, _, _ := strings.Cut(identifier, "|")
		store.Put(typeName, identifier, map[string]any{"DataShareArn": arn, dataShareInternalMarker: "INTERNAL"})
	}
}

var dataShareAuthorization = map[string]any{
	"DataShareArn":       dataShareSales,
	"ConsumerIdentifier": dataShareConsumerAccount,
	"AllowWrites":        false,
}

func Test_Suite_DataShareAuthorization(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    dataShareAuthorizationType,
		Schema:      dataShareSchema(t, "aws-redshift-datashareauthorization.json"),
		Metadata:    dataShareSchema(t, "aws-redshift-datashareauthorization.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewDataShareAuthorization(newDataShareFake(store, dataShareProducer))
		},
		Properties:       dataShareAuthorization,
		Identifier:       dataShareSales + "|" + dataShareConsumerAccount,
		Changed:          dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": true}),
		CreateOnlyChange: dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": dataShareOtherConsumer}),
		IdentifierProperties: func(identifier string) map[string]any {
			arn, consumer, _ := strings.Cut(identifier, "|")
			return map[string]any{"DataShareArn": arn, "ConsumerIdentifier": consumer}
		},
		OtherProduct: dataShareMarkInternal(dataShareAuthorizationType),
		Invalid: []map[string]any{
			dataShareWith(dataShareAuthorization, map[string]any{"DataShareArn": "sales"}),
			dataShareWith(dataShareAuthorization, map[string]any{"DataShareArn": dataShareSales + "|x"}),
			dataShareWith(dataShareAuthorization, map[string]any{"DataShareArn": 5}),
			dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": "444455556666 ADX"}),
			dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": "ADX"}),
			dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": "DataCatalog/" + dataShareConsumerAccount}),
			dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": ""}),
			dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": "true"}),
		},
		API: dataShareAuthorizationAPI,
	})
}

var dataShareConsumerAssociation = map[string]any{
	"DataShareArn": dataShareSales,
	"ConsumerArn":  dataShareNamespace,
	"AllowWrites":  false,
}

// newDataShareConsumerFake returns a fake for the consumer account, with the data shares sales and orders authorized
// for it.
func newDataShareConsumerFake(store *awsfake.CloudControl) *dataShareFake {
	for _, arn := range []string{dataShareSales, dataShareOrders} {
		store.Put(dataShareAuthorizationType, arn+"|"+dataShareConsumerAccount, map[string]any{
			"DataShareArn": arn, "ConsumerIdentifier": dataShareConsumerAccount, "AllowWrites": false,
		})
	}
	return newDataShareFake(store, dataShareConsumerAccount)
}

func Test_Suite_DataShareConsumerAssociation(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    dataShareConsumerType,
		Schema:      dataShareSchema(t, "aws-redshift-datashareconsumerassociation.json"),
		Metadata:    dataShareSchema(t, "aws-redshift-datashareconsumerassociation.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewDataShareConsumerAssociation(newDataShareConsumerFake(store))
		},
		Properties:       dataShareConsumerAssociation,
		Identifier:       dataShareSales + "|" + dataShareNamespace,
		Changed:          dataShareWith(dataShareConsumerAssociation, map[string]any{"AllowWrites": true}),
		CreateOnlyChange: dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerArn": dataShareCluster}),
		IdentifierProperties: func(identifier string) map[string]any {
			arn, last, _ := strings.Cut(identifier, "|")
			return map[string]any{"DataShareArn": arn, "ConsumerArn": last}
		},
		OtherProduct: dataShareMarkInternal(dataShareConsumerType),
		Invalid: []map[string]any{
			dataShareWith(dataShareConsumerAssociation, map[string]any{"DataShareArn": "sales"}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerArn": nil}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"AssociateEntireAccount": true}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerRegion": "us-west-2"}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerArn": "arn:aws:glue:us-west-2:444455556666:catalog"}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerArn": nil, "ConsumerRegion": "West US"}),
			dataShareWith(dataShareConsumerAssociation, map[string]any{"AssociateEntireAccount": "yes"}),
		},
		API: dataShareConsumerAPI,
	})
}

func Test_DataShareInRegion(t *testing.T) {
	opts := redshift.Options{}
	dataShareInRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
