// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

import (
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/gosuri/uiprogress"
)

type InteractiveListener struct {
	UpdateChannel <-chan clients.DeploymentProgressUpdate
	DoneChannel   chan<- struct{}
}

func (d InteractiveListener) Start() {
	go func() {
		progress := uiprogress.New()
		progress.Start()
		defer func() {
			progress.Stop()
			close(d.DoneChannel)
		}()

		bars := map[string]*uiprogress.Bar{}
		for update := range d.UpdateChannel {
			if !ShowResource(update.Resource) {
				continue
			}

			switch update.Kind {
			case clients.UpdateStart:
				resourceType := FormatTypeForDisplay(update.Resource)
				resourceName := update.Resource.Name()
				bars[update.Resource.ID] = progress.AddBar(100).
					PrependFunc(func(b *uiprogress.Bar) string {
						return fmt.Sprintf("%-20s %-15s", resourceType, resourceName)
					})

			case clients.UpdateSucceeded:
				bar := bars[update.Resource.ID]
				if bar != nil {
					bar.AppendFunc(func(b *uiprogress.Bar) string {
						return "Succeeded"
					}).Set(100)
				}

			case clients.UpdateFailed:
				bar := bars[update.Resource.ID]
				if bar != nil {
					bar.AppendFunc(func(b *uiprogress.Bar) string {
						return "Failed"
					}).Set(100)
				}
			}
		}
	}()
}

type TextListener struct {
	UpdateChannel <-chan clients.DeploymentProgressUpdate
	DoneChannel   chan<- struct{}
}

func (d TextListener) Start() {
	go func() {
		for update := range d.UpdateChannel {
			fmt.Printf("%s %s: %s\n", FormatTypeForDisplay(update.Resource), update.Resource.Name(), update.Kind)
		}

		close(d.DoneChannel)
	}()
}

func ShowResource(id azresources.ResourceID) bool {
	if len(id.Types) == 1 && id.Types[0].Name == "radiusv3" {
		// Hide operations on the provider (custom action)
		return false
	}

	return true
}

func FormatTypeForDisplay(id azresources.ResourceID) string {
	if len(id.Types) > 0 && id.Types[0].Name == "radiusv3" {
		// It's a Radius type - just use the last segment.
		return id.Types[len(id.Types)-1].Type
	}

	// It's an ARM resource, use the qualified type.
	return id.Type()
}
