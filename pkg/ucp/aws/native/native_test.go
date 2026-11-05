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

package native_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

func Test_NewRegistry(t *testing.T) {
	store := awsfake.NewCloudControl()
	catalog := schemas.Catalog{
		Native:      map[string]string{"AWS::DocDB::DBCluster": "native schema"},
		HandWritten: map[string]string{"AWS::S3::Object": "hand-written schema"},
	}

	registry, err := native.NewRegistry(catalog, nativefake.New("AWS::DocDB::DBCluster", store), nativefake.New("aws::s3::object", store))
	require.NoError(t, err)

	docdb := registry.Lookup("aws::docdb::dbcluster")
	require.NotNil(t, docdb)
	require.Equal(t, "native schema", docdb.Schema)
	require.False(t, docdb.HandWritten)

	object := registry.Lookup("AWS::S3::Object")
	require.NotNil(t, object)
	require.Equal(t, "hand-written schema", object.Schema)
	require.True(t, object.HandWritten)

	require.Nil(t, registry.Lookup("AWS::EC2::VPC"))
	require.Nil(t, (*native.Registry)(nil).Lookup("AWS::S3::Object"))

	_, err = native.NewRegistry(catalog, nativefake.New("AWS::EC2::VPC", store))
	require.EqualError(t, err, "the native handler for the AWS type AWS::EC2::VPC has no schema in pkg/aws/schemas")

	_, err = native.NewRegistry(catalog, nativefake.New("AWS::S3::Object", store), nativefake.New("AWS::S3::OBJECT", store))
	require.EqualError(t, err, "two native handlers serve the AWS type AWS::S3::OBJECT")
}

func Test_Options_Timeout(t *testing.T) {
	opts := native.Options{CreateTimeout: time.Minute, DeleteTimeout: time.Second}
	require.Equal(t, time.Minute, opts.Timeout(native.OperationCreate))
	require.Equal(t, native.DefaultTimeout, opts.Timeout(native.OperationUpdate))
	require.Equal(t, time.Second, opts.Timeout(native.OperationDelete))
	require.Equal(t, native.DefaultTimeout, opts.Timeout("UNKNOWN"))
}

func Test_Failure(t *testing.T) {
	failure := &native.Failure{Message: "bad value", ClientError: true}
	require.Equal(t, "bad value", failure.Error())
	require.True(t, failure.IsClientError())
}
