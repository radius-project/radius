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

// Package records holds execution records and operations (Stack B): the grants UCP
// creates when it authorizes a user request, and the per-resource operations it assigns
// to a component.
package records

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// HeaderRecord carries the execution record ID on internal calls.
	HeaderRecord = "X-Radius-Execution-Record"

	// HeaderOperation carries the operation ID on internal calls.
	HeaderOperation = "X-Radius-Operation-Id"

	// HeaderSourceNamespace carries the Kubernetes namespace the controller acts for.
	HeaderSourceNamespace = "X-Radius-Source-Namespace"

	// RootScope is where records and operations are stored.
	RootScope = "/planes/radius/local"

	// RecordResourceType is the resource type of execution records.
	RecordResourceType = "System.Authorization/executionRecords"

	// OperationResourceType is the resource type of operations.
	OperationResourceType = "System.Authorization/operations"

	// DefaultRecordTTL is how long a record stays active if nothing closes it.
	DefaultRecordTTL = time.Hour

	// DefaultLease is how long a worker's claim on an operation lasts.
	DefaultLease = 15 * time.Minute

	// ActionAll matches every action.
	ActionAll = "*"
)

// RecordStatus is the status of an execution record.
type RecordStatus string

const (
	RecordActive  RecordStatus = "active"
	RecordClosed  RecordStatus = "closed"
	RecordRevoked RecordStatus = "revoked"
)

// OperationStatus is the status of an operation.
type OperationStatus string

const (
	OperationQueued    OperationStatus = "queued"
	OperationRunning   OperationStatus = "running"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
	OperationCancelled OperationStatus = "cancelled"
)

// Record is an execution record: what a subject allowed a submitter to do, where, and until when.
type Record struct {
	UserApproval *UserApproval `json:"userApproval,omitempty"`
	ID           string        `json:"id"`
	Subject      string        `json:"subject"`
	Submitter    string        `json:"submitter"`
	Actions      []string      `json:"actions"`
	Targets      []string      `json:"targets"`
	Application  string        `json:"application,omitempty"`
	Environment  string        `json:"environment,omitempty"`
	ParentID     string        `json:"parentRecordId,omitempty"`
	Deployment   string        `json:"deployment,omitempty"`
	Status       RecordStatus  `json:"status"`
	CreatedAt    time.Time     `json:"createdAt"`
	ExpiresAt    time.Time     `json:"expiresAt"`
	ClosedAt     *time.Time    `json:"closedAt,omitempty"`
}

// Active is true when the record is active and not expired.
func (r *Record) Active(now time.Time) bool {
	return r.Status == RecordActive && now.Before(r.ExpiresAt)
}

// Covers is true when action on resourceID is within the record's actions and targets.
func (r *Record) Covers(action, resourceID string) bool {
	return r.CoversAction(action) && r.CoversTarget(resourceID)
}

// CoversAction is true when action matches one of the record's actions.
func (r *Record) CoversAction(action string) bool {
	for _, a := range r.Actions {
		if a == ActionAll || strings.EqualFold(a, action) {
			return true
		}
	}
	return false
}

// CoversTarget is true when resourceID is within one of the record's targets.
func (r *Record) CoversTarget(resourceID string) bool {
	for _, t := range r.Targets {
		if ScopeCovers(t, resourceID) {
			return true
		}
	}
	return false
}

// IsSubsetOf is true when every action and target of r is covered by parent.
func (r *Record) IsSubsetOf(parent *Record) bool {
	for _, a := range r.Actions {
		if a == ActionAll {
			if !parent.CoversAction(ActionAll) {
				return false
			}
			continue
		}
		if !parent.CoversAction(a) {
			return false
		}
	}
	for _, t := range r.Targets {
		if !parent.CoversTarget(t) {
			return false
		}
	}
	return len(r.Targets) > 0
}

// Operation is one write that UCP assigned to one component under a record.
type Operation struct {
	ID                string          `json:"id"`
	RecordID          string          `json:"recordId"`
	ResourceID        string          `json:"resourceId"`
	Action            string          `json:"action"`
	AssignedComponent string          `json:"assignedComponent"`
	InputHash         string          `json:"inputHash"`
	Status            OperationStatus `json:"status"`
	LeaseHolder       string          `json:"leaseHolder,omitempty"`
	LeaseExpiresAt    *time.Time      `json:"leaseExpiresAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// Open is true while the operation may still run.
func (o *Operation) Open() bool {
	return o.Status == OperationQueued || o.Status == OperationRunning
}

// ScopeCovers is true when id equals scope or is inside it. Comparison ignores case
// and respects segment boundaries.
func ScopeCovers(scope, id string) bool {
	s := strings.TrimSuffix(strings.ToLower(scope), "/")
	i := strings.TrimSuffix(strings.ToLower(id), "/")
	if s == "" {
		return false
	}
	return i == s || strings.HasPrefix(i, s+"/")
}

// SameResource compares two resource IDs, ignoring case.
func SameResource(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "/"), strings.TrimSuffix(b, "/"))
}

// ActionFor returns the action for an HTTP method on a resource type, such as
// "Applications.Core/containers/write". It returns "" for reads.
func ActionFor(method, resourceType string) string {
	switch strings.ToUpper(method) {
	case http.MethodPut, http.MethodPatch:
		return resourceType + "/write"
	case http.MethodDelete:
		return resourceType + "/delete"
	case http.MethodPost:
		return resourceType + "/action"
	default:
		return ""
	}
}

// IsWrite is true for methods that change state and need an operation.
func IsWrite(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// InputHash returns the SHA-256 of the canonical JSON form of body. Whitespace and key
// order do not change the hash. An empty body hashes to "". A body that is not JSON is
// hashed as-is.
func InputHash(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	canonical := trimmed
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err == nil {
		if b, err := json.Marshal(v); err == nil {
			canonical = b
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// NewRecordID returns a new record resource ID.
func NewRecordID() string {
	return RecordID(uuid.NewString())
}

// RecordID returns the resource ID of the record named name.
func RecordID(name string) string {
	return RootScope + "/providers/" + RecordResourceType + "/" + name
}

// NewOperationID returns a new operation resource ID.
func NewOperationID() string {
	return RootScope + "/providers/" + OperationResourceType + "/" + uuid.NewString()
}

// ResourceGroupScope returns the resource group scope of id, or its root scope when it
// is not in a resource group.
func ResourceGroupScope(id resources.ID) string {
	for i, segment := range id.ScopeSegments() {
		if strings.EqualFold(segment.Type, "resourcegroups") {
			parts := id.ScopeSegments()[:i+1]
			scope := ""
			for _, p := range parts {
				scope += "/" + p.Type + "/" + p.Name
			}
			return "/planes" + scope
		}
	}
	return id.RootScope()
}
