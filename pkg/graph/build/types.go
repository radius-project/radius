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

package build

// Direction names match the corerp Direction enum string values so that
// downstream consumers built against the generated client can interoperate.
const (
	DirectionOutbound = "Outbound"
	DirectionInbound  = "Inbound"
)

// StaticGraphArtifact is the JSON envelope for the static graph artifact.
// When using orphan branch storage, this is committed to
// {source-branch}/app.json on the radius-graph orphan branch.
type StaticGraphArtifact struct {
	// Version is the schema version of the artifact (e.g. "1.0.0").
	Version string `json:"version"`

	// GeneratedAt is the RFC 3339 timestamp at which the artifact was produced.
	GeneratedAt string `json:"generatedAt"`

	// SourceFile is the repo-relative, slash-style path to the Bicep file that
	// the artifact was generated from.
	SourceFile string `json:"sourceFile"`

	// Application is the application graph payload.
	Application Application `json:"application"`
}

// Application describes the application graph.
type Application struct {
	// Resources are the nodes of the application graph, sorted by ID for
	// deterministic output.
	Resources []*Resource `json:"resources"`
}

// Resource is a node in the static application graph. It mirrors the relevant
// fields of corerp.ApplicationGraphResource and additionally carries
// review-time metadata (CodeReference, AppDefinitionLine, DiffHash).
type Resource struct {
	// ID is the fully qualified Radius resource ID.
	ID string `json:"id"`

	// Type is the resource type without the API version
	// (e.g. "Applications.Core/containers").
	Type string `json:"type"`

	// Name is the resource name.
	Name string `json:"name"`

	// ProvisioningState reflects the deployment state of the resource.
	// Static graphs always report "Succeeded".
	ProvisioningState string `json:"provisioningState"`

	// CodeReference is an optional pointer back to the source location
	// (e.g. "src/frontend/index.ts#L1") authored on the Bicep resource.
	CodeReference string `json:"codeReference,omitempty"`

	// AppDefinitionLine is the 1-based line number in the source Bicep file
	// where the resource is declared. Zero when unknown.
	AppDefinitionLine int32 `json:"appDefinitionLine,omitempty"`

	// DiffHash is a content hash over the review-relevant authorable
	// properties of the resource, used to detect meaningful changes between
	// successive static graphs.
	DiffHash string `json:"diffHash,omitempty"`

	// Connections are the directed edges out of (and into) this resource.
	Connections []*Connection `json:"connections"`

	// OutputResources are the backing concrete resources that comprise this
	// resource (populated by runtime graphs; empty for static graphs).
	OutputResources []*OutputResource `json:"outputResources"`
}

// Connection is a directed edge between two Resources.
type Connection struct {
	// ID is the resource ID of the other endpoint of the edge. The current
	// resource is implied by context.
	ID string `json:"id"`

	// Direction is DirectionOutbound or DirectionInbound and indicates whether
	// ID is the destination or the source of the edge respectively.
	Direction string `json:"direction"`
}

// OutputResource describes a backing concrete resource.
type OutputResource struct {
	// ID is the fully qualified resource ID of the backing resource.
	ID string `json:"id"`

	// Name is the backing resource name.
	Name string `json:"name"`

	// Type is the backing resource type (e.g. "apps/Deployment").
	Type string `json:"type"`
}
