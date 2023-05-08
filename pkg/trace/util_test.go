/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
