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

package style

import (
	"image/color"
	"sync"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/radius-project/radius/pkg/process"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveColor_NoWindowUsesDarkColorWithoutTerminalQuery(t *testing.T) {
	t.Setenv(process.NoWindowEnvVar, "true")
	resetBackgroundDetection(t, func() bool {
		t.Fatal("terminal background must not be queried in no-window mode")
		return false
	})

	color := AdaptiveColor{
		Light: lipgloss.Color("#111111"),
		Dark:  lipgloss.Color("#EEEEEE"),
	}

	require.Equal(t, rgba(lipgloss.Color("#EEEEEE")), rgba(color))
}

func TestAdaptiveColor_UsesDetectedBackground(t *testing.T) {
	t.Setenv(process.NoWindowEnvVar, "")
	resetBackgroundDetection(t, func() bool {
		return false
	})

	color := AdaptiveColor{
		Light: lipgloss.Color("#111111"),
		Dark:  lipgloss.Color("#EEEEEE"),
	}

	require.Equal(t, rgba(lipgloss.Color("#111111")), rgba(color))
}

func rgba(value color.Color) [4]uint32 {
	red, green, blue, alpha := value.RGBA()
	return [4]uint32{red, green, blue, alpha}
}

func resetBackgroundDetection(t *testing.T, detect func() bool) {
	t.Helper()
	originalDetect := detectDarkBackground
	backgroundDetection = sync.Once{}
	hasDarkBackground = false
	detectDarkBackground = detect
	t.Cleanup(func() {
		backgroundDetection = sync.Once{}
		hasDarkBackground = false
		detectDarkBackground = originalDetect
	})
}
