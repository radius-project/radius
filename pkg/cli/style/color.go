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

// Package style provides shared CLI styling that is safe for non-interactive processes.
package style

import (
	"image/color"
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

var (
	backgroundDetection sync.Once
	hasDarkBackground   bool
	hasTerminal         = func() bool {
		return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
	}
	detectDarkBackground = func() bool {
		return lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	}
)

// AdaptiveColor selects a color for the terminal background when the color is rendered.
type AdaptiveColor struct {
	Light color.Color
	Dark  color.Color
}

// RGBA returns the color selected for the current terminal background.
func (c AdaptiveColor) RGBA() (uint32, uint32, uint32, uint32) {
	backgroundDetection.Do(func() {
		if !hasTerminal() {
			hasDarkBackground = true
			return
		}
		hasDarkBackground = detectDarkBackground()
	})

	if hasDarkBackground {
		return c.Dark.RGBA()
	}
	return c.Light.RGBA()
}
