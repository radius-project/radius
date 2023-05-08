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
