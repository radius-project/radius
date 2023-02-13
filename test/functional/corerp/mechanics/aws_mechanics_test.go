// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_AWSRedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	templateFmt := "testdata/aws-mechanics-redeploy-withupdatedresource.step%d.bicep"
	name := "ms" + uuid.New().String()

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.KinesisResourceType,
						Identifier: name,
						Properties: map[string]any{
							"Name":                 name,
							"RetentionPeriodHours": float64(24),
							"ShardCount":           float64(3),
						},
					},
				},
			},
		},
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.KinesisResourceType,
						Identifier: name,
						Properties: map[string]any{
							"Name":                 name,
							"RetentionPeriodHours": float64(48),
							"ShardCount":           float64(3),
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

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1)),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.DBInstanceResourceType,
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
			Executor:                               step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2)),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.DBInstanceResourceType,
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
