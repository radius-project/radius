// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package trace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestExtractTraceparent(t *testing.T) {
	traceparentTests := []string{
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		"00-10000000000000000000000000000000-00f067aa0ba902b7-00",
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	for _, traceparent := range traceparentTests {
		t.Run(traceparent, func(t *testing.T) {
			ctx := WithTraceparent(context.TODO(), traceparent)

			tp := ExtractTraceparent(ctx)
			require.Equal(t, traceparent, tp)
		})
	}
}
