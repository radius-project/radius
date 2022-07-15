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
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
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
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/rabbitMQMessageQueues/queue0",
			Name: "queue0",
			Type: "Applications.Connector/rabbitMQMessageQueues",
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			RabbitMQMessageQueueResponseProperties: datamodel.RabbitMQMessageQueueResponseProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Queue:       "abc",
			},
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
	expectedSecretValues := map[string]rp.SecretValueReference{
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
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/rabbitMQMessageQueues/queue0",
			Name: "queue0",
			Type: "Applications.Connector/rabbitMQMessageQueues",
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			RabbitMQMessageQueueResponseProperties: datamodel.RabbitMQMessageQueueResponseProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Secrets: datamodel.RabbitMQSecrets{ConnectionString: "admin:deadbeef@localhost:42"},
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, "queue name must be specified", err.Error())
}
