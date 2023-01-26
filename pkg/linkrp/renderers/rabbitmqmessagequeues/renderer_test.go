// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqmessagequeues

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_User_Secrets(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.RabbitMQMessageQueue{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/rabbitMQMessageQueues/queue0",
				Name: "queue0",
				Type: linkrp.RabbitMQMessageQueuesResourceType,
			},
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Queue:   "abc",
			Secrets: datamodel.RabbitMQSecrets{ConnectionString: "admin:deadbeef@localhost:42"},
		},
	}

	output, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.NoError(t, err)

	require.Len(t, output.Resources, 0)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		QueueNameKey: {
			Value: "abc",
		},
	}
	expectedSecretValues := map[string]rpv1.SecretValueReference{
		"connectionString": {
			Value: "admin:deadbeef@localhost:42",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, expectedSecretValues, output.SecretValues)
	require.Equal(t, 1, len(output.SecretValues))
}

func Test_Render_NoQueueSpecified(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.RabbitMQMessageQueue{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/rabbitMQMessageQueues/queue0",
				Name: "queue0",
				Type: linkrp.RabbitMQMessageQueuesResourceType,
			},
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Secrets: datamodel.RabbitMQSecrets{ConnectionString: "admin:deadbeef@localhost:42"},
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "queue name must be specified", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.RabbitMQMessageQueue{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/rabbitMQMessageQueues/queue0",
				Name: "queue0",
				Type: linkrp.RabbitMQMessageQueuesResourceType,
			},
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "invalid-app-id",
			},
			Queue:   "abc",
			Secrets: datamodel.RabbitMQSecrets{ConnectionString: "admin:deadbeef@localhost:42"},
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}
