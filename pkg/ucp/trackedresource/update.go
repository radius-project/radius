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

package trackedresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	retryCount     = 10
	retryDelay     = time.Second * 3
	requestTimeout = time.Second * 10
)

// NewUpdater creates a new Updater.
func NewUpdater(databaseClient database.Client, httpClient *http.Client) *Updater {
	return &Updater{
		DatabaseClient: databaseClient,
		Client:         httpClient,
		AttemptCount:   retryCount,
		RetryDelay:     retryDelay,
		RequestTimeout: requestTimeout,
	}
}

// Updater is a utility struct that can perform updates on tracked resources.
type Updater struct {
	// DatabaseClient is the database client used to access the database.
	DatabaseClient database.Client

	// Client is the HTTP client used to make requests to the downstream API.
	Client *http.Client

	// AttemptCount is the number of times to attempt a request and database update.
	AttemptCount int

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration

	// RequestTimeout is the timeout used for requests to the downstream API.
	RequestTimeout time.Duration
}

// InProgressErr signifies that the resource is currently in a non-terminal state.
type InProgressErr struct {
}

// Error returns the error message.
func (e *InProgressErr) Error() string {
	return "resource is still being provisioned"
}

// Is returns true if the other error is an InProgressErr.
func (e *InProgressErr) Is(other error) bool {
	_, ok := other.(*InProgressErr)
	return ok
}

// trackedResourceState holds the state of a tracked resource as reported by the downstream API.
// This only defines the fields we use, so many fields returned by the API are omitted.
type trackedResourceState struct {
	ID         string                         `json:"id"`
	Name       string                         `json:"name"`
	Type       string                         `json:"type"`
	Properties trackedResourceStateProperties `json:"properties,omitempty"`
}

type trackedResourceStateProperties struct {
	ProvisioningState *v1.ProvisioningState `json:"provisioningState,omitempty"`
}

// Update updates a tracked resource.
//
// This function return attempt to update the state using optimistic concurrency and will retry on the following
// conditions:
//
// - Downstream failure or timeout
// - Database failure
// - Optimistic concurrency failure
// - Resource is still being provisioned (provisioning state is non-terminal)
func (u *Updater) Update(ctx context.Context, downstream string, id resources.ID, apiVersion string) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	destination, err := url.Parse(downstream)
	if err != nil {
		return err
	}

	destination = destination.JoinPath(id.String())

	query := destination.Query()
	query.Set("api-version", apiVersion)
	destination.RawQuery = query.Encode()

	// Tracking ID is the ID of the TrackedResourceEntry that will store the data.
	//
	// Example:
	//	id: /planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app
	//	trackingID: /planes/radius/local/resourceGroups/test-group/providers/System.Resources/trackingResourceEntries/test-app-ec291e26078b7ea8a74abfac82530005a0ecbf15
	trackingID := IDFor(id)

	logger = logger.WithValues("id", id, "trackingID", trackingID, "destination", destination.String())
	logger.V(ucplog.LevelDebug).Info("updating tracked resource")
	for attempt := 1; attempt <= u.AttemptCount; attempt++ {
		logger.WithValues("attempt", attempt)
		ctx := logr.NewContext(ctx, logger)
		logger.V(ucplog.LevelDebug).Info("beginning attempt")

		err := u.run(ctx, id, trackingID, destination, apiVersion)
		if errors.Is(err, &InProgressErr{}) && attempt == u.AttemptCount {
			// Preserve the InprogressErr for the last attempt.
			return err
		} else if err != nil {
			logger.Error(err, "attempt failed", "delay", u.RetryDelay)
			time.Sleep(u.RetryDelay)
			continue
		}

		logger.V(ucplog.LevelDebug).Info("tracked resource processing completed successfully")
		return nil
	}

	return fmt.Errorf("failed to update tracked resource after %d attempts", u.AttemptCount)
}

func (u *Updater) run(ctx context.Context, id resources.ID, trackingID resources.ID, destination *url.URL, apiVersion string) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	obj, err := u.DatabaseClient.Get(ctx, trackingID.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		// This is fine. It might be a new resource.
	} else if err != nil {
		return err
	}

	etag := ""
	entry := datamodel.GenericResourceFromID(id, trackingID)
	entry.Properties.APIVersion = apiVersion
	if obj != nil {
		etag = obj.ETag
		err := obj.As(&entry)
		if err != nil {
			return err
		}
	}

	data, err := u.fetch(ctx, destination)
	if err != nil {
		return err
	}

	if data == nil {
		// Resource was not found. We can delete the tracked resource entry.
		logger.V(ucplog.LevelDebug).Info("deleting tracked resource entry")
		err = u.DatabaseClient.Delete(ctx, trackingID.String(), database.WithETag(etag))
		if errors.Is(err, &database.ErrNotFound{}) {
			return nil
		} else if err != nil {
			return err
		}

		return nil
	} else if data.Properties.ProvisioningState != nil && !data.Properties.ProvisioningState.IsTerminal() {
		// Resource is still being provisioned. We should not update anything yet.
		logger.V(ucplog.LevelDebug).Info("resource is still being provisioned")
		return &InProgressErr{}
	}

	// If we get here we're ready to save the changes for a create/update.
	//
	// Mark the resource as provisioned. This will will "reset" the lock on the resource.
	entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
	if data.Properties.ProvisioningState != nil {
		entry.AsyncProvisioningState = *data.Properties.ProvisioningState
	}

	obj = &database.Object{
		Metadata: database.Metadata{
			ID: trackingID.String(),
		},
		Data: entry,
	}
	logger.V(ucplog.LevelDebug).Info("updating tracked resource entry")
	err = u.DatabaseClient.Save(ctx, obj, database.WithETag(etag))
	if errors.Is(err, &database.ErrConcurrency{}) {
		logger.V(ucplog.LevelDebug).Info("tracked resource was updated concurrently")
		return &InProgressErr{}
	} else if err != nil {
		return err
	}

	return nil
}

func (u *Updater) fetch(ctx context.Context, destination *url.URL) (*trackedResourceState, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	logger.V(ucplog.LevelDebug).Info("fetching resource")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, destination.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := u.Client.Do(request)
	if err != nil {
		return nil, err
	}
	logger.V(ucplog.LevelDebug).Info("resource fetched", "status", response.StatusCode)

	defer response.Body.Close()
	if !u.isJSONResponse(response) {
		return nil, fmt.Errorf("response is not JSON. Content-Type: %q", response.Header.Get("Content-Type"))
	}

	if response.StatusCode == 404 {
		return nil, nil
	}

	if response.StatusCode >= 400 {
		return nil, u.reportRequestFailure(response)
	}

	data := &trackedResourceState{}
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (u *Updater) isJSONResponse(response *http.Response) bool {
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return false
	}

	if contentType == "application/json" {
		return true
	} else if contentType == "text/json" {
		return true
	} else if strings.HasSuffix(contentType, "+json") {
		return true
	}

	return false
}

func (u *Updater) reportRequestFailure(response *http.Response) error {
	data := v1.ErrorResponse{}

	decoder := json.NewDecoder(response.Body)
	err := decoder.Decode(&data)
	if err != nil {
		return err
	}

	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return fmt.Errorf("request failed with status code %s:\n%s", response.Status, body)
}
