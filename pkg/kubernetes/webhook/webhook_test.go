package webhook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Webhook_MustHaveValidatorPathOrMutatingPath(t *testing.T) {

	webhook := NewGenericWebhookManagedBy(nil)

	err := webhook.Complete(nil)

	require.Error(t, err)
	require.Equal(t, "validatePath or mutatePath must be set", err.Error())
}
