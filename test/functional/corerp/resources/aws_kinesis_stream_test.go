// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_AWS_KinesisStream(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	name := "ms" + uuid.New().String()

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.KinesisResourceType,
						Identifier: name,
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_KinesisStreamExisting(t *testing.T) {
	template := "testdata/aws-kinesis.bicep"
	templateExisting := "testdata/aws-kinesis-existing.bicep"
	name := "ms" + uuid.New().String()
	appNamespace := "default-aws-kinesis-existing-app"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:                               step.NewDeployExecutor(template, "streamName="+name),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			AWSResources: &validation.AWSResourceSet{
				Resources: []validation.AWSResource{
					{
						Name:       name,
						Type:       validation.KinesisResourceType,
						Identifier: name,
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(templateExisting, "streamName="+name, functional.GetMagpieImage()),
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
					appNamespace: {
						validation.NewK8sPodForResource("aws-kinesis-existing-app", "aws-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				labelset := kubernetes.MakeSelectorLabels("aws-kinesis-existing-app", "aws-ctnr")

				deployments, err := ct.Options.K8sClient.AppsV1().Deployments(appNamespace).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list deployments")
				require.Len(t, deployments.Items, 1, "expected 1 deployment")
				deployment := deployments.Items[0]
				envVar := deployment.Spec.Template.Spec.Containers[0].Env[0]
				require.Equal(t, "TEST", envVar.Name, "expected env var to be updated")
				require.Equal(t, name, envVar.Value, "expected env var to be updated")
			},
		},
	})

	test.Test(t)

}
