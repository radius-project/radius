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

package mechanics_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_AWSRedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	templateFmt := "testdata/aws-mechanics-redeploy-withupdatedresource.step%d.bicep"
	name := "radiusfunctionaltestbucket-" + uuid.New().String()
	creationTimestamp := functional.GetCreationTimestamp()

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: name,
						Properties: map[string]any{
							"BucketName": name,
							"Tags": []any{
								map[string]any{
									"Key":   "testKey",
									"Value": "testValue",
								},
							},
						},
					},
				},
			},
		},
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), "bucketName="+name, "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSS3BucketResourceType,
						Identifier: name,
						Properties: map[string]any{
							"BucketName": name,
							"Tags": []any{
								map[string]any{
									"Key":   "testKey",
									"Value": "testValue2",
								},
							},
						},
					},
				},
			},
		},
	})
	test.Test(t)
}

func Test_AWSRedeployWithCreateAndWriteOnlyPropertyUpdate(t *testing.T) {
	t.Skip("This test will fail because step 2 is updating a create-and-write-only property.")
	name := "my-db"
	templateFmt := "testdata/aws-mechanics-redeploy-withcreateandwriteonlypropertyupdate.step%d.bicep"
	creationTimestamp := functional.GetCreationTimestamp()

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSRDSDBInstanceResourceType,
						Identifier: name,
						Properties: map[string]any{
							"Endpoint": map[string]any{
								"Port": 1444,
							},
						},
					},
				},
			},
		},
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), "creationTimestamp="+creationTimestamp),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.AWSRDSDBInstanceResourceType,
						Identifier: name,
						Properties: map[string]any{
							"Endpoint": map[string]any{
								"Port": 1444,
							},
						},
					},
				},
			},
		},
	})
	test.Test(t)
}
