// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volume

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/stretchr/testify/require"
)

func TestRender_NotSupported(t *testing.T) {
	r := NewRenderer(nil)
	vol := &datamodel.VolumeResource{
		Properties: datamodel.VolumeResourceProperties{
			Kind: "fakevol",
		},
	}

	_, err := r.Render(context.Background(), vol, renderers.RenderOptions{
		Environment: renderers.EnvironmentOptions{
			Namespace: "default",
		},
	})

	require.Error(t, err)
	require.Equal(t, "fakevol is not supported", err.Error())
}
