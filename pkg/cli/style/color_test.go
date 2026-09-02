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
	"github.com/stretchr/testify/require"
)

func TestAdaptiveColor_NoTerminalUsesDarkColorWithoutTerminalQuery(t *testing.T) {
	resetBackgroundDetection(t, false, func() bool {
		t.Fatal("terminal background must not be queried without a terminal")
		return false
	})

	color := AdaptiveColor{
		Light: lipgloss.Color("#111111"),
		Dark:  lipgloss.Color("#EEEEEE"),
	}

	require.Equal(t, rgba(lipgloss.Color("#EEEEEE")), rgba(color))
}

func TestAdaptiveColor_UsesDetectedBackground(t *testing.T) {
	resetBackgroundDetection(t, true, func() bool {
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

func resetBackgroundDetection(t *testing.T, terminal bool, detect func() bool) {
	t.Helper()
	originalHasTerminal := hasTerminal
	originalDetect := detectDarkBackground
	backgroundDetection = sync.Once{}
	hasDarkBackground = false
	hasTerminal = func() bool {
		return terminal
	}
	detectDarkBackground = detect
	t.Cleanup(func() {
		backgroundDetection = sync.Once{}
		hasDarkBackground = false
		hasTerminal = originalHasTerminal
		detectDarkBackground = originalDetect
	})
}
