// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_KinesisStream(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	name := "ms" + uuid.New().String()

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.KinesisResourceType,
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_KinesisStreamExisting(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	templateExisting := "testdata/aws-kinesis-existing.bicep"
	name := "ms" + uuid.New().String()

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.KinesisResourceType,
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(templateExisting, "streamName="+name, functional.GetMagpieImage()),
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name: name,
						Type: validation.KinesisResourceType,
					},
				},
			},
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "aws-kinesis-existing-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "aws-ctnr",
						Type: validation.ContainersResource,
						App:  "aws-kinesis-existing-app",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource("aws-kinesis-existing-app", "aws-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				deployments, err := ct.Options.K8sClient.AppsV1().Deployments("default").List(context.Background(), metav1.ListOptions{})
				require.NoError(t, err, "failed to list deployments")
				require.Len(t, deployments.Items, 1, "expected 1 deployment")
				deployment := deployments.Items[0]
				envVar := deployment.Spec.Template.Spec.Containers[0].Env[0]
				require.Equal(t, "TEST", envVar.Name, "expected env var to be updated")
				require.Equal(t, name, envVar.Value, "expected env var to be updated")
			},
		},
	}, requiredSecrets)

	test.Test(t)

}
