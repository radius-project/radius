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

// Package dataaccess sends a resource provider's database writes through UCP's
// data-access endpoint (D2, D3). Reads stay direct.
package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
)

// Client is a database client whose Save and Delete calls go to UCP.
type Client struct {
	database.Client

	// Endpoint is the UCP base URL, including the path base.
	Endpoint string

	// HTTP sends requests to UCP. Its transport presents this component's certificate
	// and adds the record and operation headers from the request context.
	HTTP *http.Client
}

// Wrapper returns a function that wraps a database client so its writes go to UCP over conn.
func Wrapper(conn sdk.Connection) func(database.Client) database.Client {
	return func(inner database.Client) database.Client {
		return &Client{Client: inner, Endpoint: strings.TrimSuffix(conn.Endpoint(), "/"), HTTP: conn.Client()}
	}
}

// Error is a write UCP refused.
type Error struct {
	StatusCode int
	Code       string
	Message    string
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("UCP data access denied (%s, %d): %s", e.Code, e.StatusCode, e.Message)
}

// direct reports whether id is authorization bookkeeping, which the component writes
// itself (a known prototype gap).
func direct(id string) bool {
	return strings.Contains(strings.ToLower(id), "/providers/system.authorization/")
}

// Save implements database.Client.
func (c *Client) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if obj == nil || direct(obj.ID) {
		return c.Client.Save(ctx, obj, options...)
	}
	cfg := database.NewSaveConfig(options...)
	var resp authzcheck.DataResponse
	err := c.post(ctx, authzcheck.DataSavePath, authzcheck.DataSaveRequest{ID: obj.ID, ETag: string(cfg.ETag), Data: obj.Data}, obj.ID, &resp)
	if err != nil {
		return err
	}
	obj.ETag = database.ETag(resp.ETag)
	return nil
}

// Delete implements database.Client.
func (c *Client) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
	if direct(id) {
		return c.Client.Delete(ctx, id, options...)
	}
	cfg := database.NewDeleteConfig(options...)
	return c.post(ctx, authzcheck.DataDeletePath, authzcheck.DataDeleteRequest{ID: id, ETag: string(cfg.ETag)}, id, nil)
}

func (c *Client) post(ctx context.Context, path string, body any, id string, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// The HTTP client's transport adds the headers from ctx. Set them here too for clients without it.
	if ids := records.IDsFromContext(ctx); ids.OperationID != "" {
		req.Header.Set(records.HeaderRecord, ids.RecordID)
		req.Header.Set(records.HeaderOperation, ids.OperationID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("UCP data access failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	case http.StatusNotFound:
		return &database.ErrNotFound{ID: id}
	case http.StatusPreconditionFailed:
		return &database.ErrConcurrency{}
	case http.StatusBadRequest:
		return &database.ErrInvalid{Message: string(data)}
	}
	var armErr v1.ErrorResponse
	_ = json.Unmarshal(data, &armErr)
	e := &Error{StatusCode: resp.StatusCode, Message: string(data)}
	if armErr.Error != nil {
		e.Code, e.Message = armErr.Error.Code, armErr.Error.Message
	}
	return e
}
