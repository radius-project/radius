/*
Copyright 2025 The Radius Authors.

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

package delete

import (
	"context"
	"sync"

	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// DeleteApplicationWithProgress deletes an application with progress updates displayed to the user.
// This is intended to be used from the CLI and thus logs to the console.
func DeleteApplicationWithProgress(ctx context.Context, amc clients.ApplicationsManagementClient, options clients.DeleteOptions) (bool, error) {
	step := output.BeginStep("%s", options.ProgressText)
	output.LogInfo("")

	progressChan := make(chan clients.ResourceProgress, 1)
	listener := NewProgressListener(progressChan)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		listener.Run()
		wg.Done()
	}()

	resourcesList, err := amc.ListResourcesInApplication(ctx, options.ApplicationNameOrID)
	if err != nil && !clientv2.Is404Error(err) {
		close(progressChan)
		wg.Wait()
		return false, err
	}

	processedResources := make(map[string]bool)

	for _, resource := range resourcesList {
		if resource.ID != nil {
			resourceID, err := resources.ParseResource(*resource.ID)
			if err == nil {
				if _, exists := processedResources[resourceID.String()]; !exists {
					progressChan <- clients.ResourceProgress{
						Resource: resourceID,
						Status:   clients.StatusStarted,
					}
					processedResources[resourceID.String()] = true
				}
			}
		}
	}

	app, err := amc.GetApplication(ctx, options.ApplicationNameOrID)
	if err == nil && app.ID != nil {
		appID, err := resources.ParseResource(*app.ID)
		if err == nil {
			if _, exists := processedResources[appID.String()]; !exists {
				progressChan <- clients.ResourceProgress{
					Resource: appID,
					Status:   clients.StatusStarted,
				}
				processedResources[appID.String()] = true
			}
		}
	}

	deleted, err := amc.DeleteApplication(ctx, options.ApplicationNameOrID)

	if err == nil {
		for _, resource := range resourcesList {
			if resource.ID != nil {
				resourceID, err := resources.ParseResource(*resource.ID)
				if err == nil {
					progressChan <- clients.ResourceProgress{
						Resource: resourceID,
						Status:   clients.StatusCompleted,
					}
				}
			}
		}

		if app.ID != nil {
			appID, err := resources.ParseResource(*app.ID)
			if err == nil {
				progressChan <- clients.ResourceProgress{
					Resource: appID,
					Status:   clients.StatusCompleted,
				}
			}
		}
	}

	close(progressChan)
	wg.Wait()

	if err != nil {
		return false, err
	}

	output.LogInfo("")
	output.CompleteStep(step)

	return deleted, nil
}
