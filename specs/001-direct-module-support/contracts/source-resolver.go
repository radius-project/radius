// Package source provides module source classification and validation for
// Terraform recipe recipe locations. It determines whether a recipeLocation
// refers to a direct Terraform module source (registry, Git, HTTP) or an
// existing OCI/wrapped recipe path, enabling the recipe system to apply
// appropriate execution and output mapping strategies.
//
// This file defines the contract (interface + types) for the source resolver.
// Implementation will be in resolver.go.
package source

import "context"

// SourceType classifies the format of a Terraform module source path.
type SourceType int

const (
	// SourceTypeUnknown indicates the source format could not be classified.
	// The system should fall back to existing OCI/wrapped recipe resolution.
	SourceTypeUnknown SourceType = iota

	// SourceTypeTerraformRegistry indicates a standard Terraform registry source.
	// Format: "namespace/name/provider" (exactly 3 slash-separated segments, no scheme).
	// Example: "hashicorp/consul/aws", "Azure/cosmosdb/azurerm"
	SourceTypeTerraformRegistry

	// SourceTypeGit indicates a Git-hosted module source.
	// Format: "git::https://..." or "git::ssh://..."
	// Supports ref specifiers (?ref=v1.0.0) and subdirectories (//modules/vpc).
	SourceTypeGit

	// SourceTypeHTTP indicates an HTTP/HTTPS archive source.
	// Format: "https://example.com/module.tar.gz" (without git:: prefix)
	SourceTypeHTTP

	// SourceTypeS3 indicates an S3-hosted module source.
	// Format: "s3::bucket-name/key"
	SourceTypeS3

	// SourceTypeGCS indicates a GCS-hosted module source.
	// Format: "gcs::bucket-name/key"
	SourceTypeGCS

	// SourceTypeOCI indicates an OCI registry source (existing wrapped recipe path).
	// Format: contains "oci://" or matches OCI image reference patterns.
	SourceTypeOCI
)

// ResolvedSource contains the classification result for a recipe location.
type ResolvedSource struct {
	// Type is the classified source type.
	Type SourceType

	// OriginalPath is the unmodified recipeLocation value.
	OriginalPath string

	// IsDirectModule is true when the source is a direct Terraform module
	// (not a wrapped/OCI recipe). This determines output mapping strategy.
	IsDirectModule bool
}

// Resolver classifies and validates Terraform module source paths.
type Resolver interface {
	// Classify determines the source type of a recipe location without making
	// any network calls. Classification is purely based on string pattern matching.
	//
	// Returns a ResolvedSource with Type set to the detected source type.
	// If the format is not recognized, Type is SourceTypeUnknown and
	// IsDirectModule is false (indicating fallback to existing behavior).
	Classify(recipeLocation string) ResolvedSource

	// ValidateReachability performs a lightweight network check to verify
	// that the module source is accessible. This is called at RecipePack
	// creation time per FR-014.
	//
	// For registry modules: HTTP GET to registry API
	// For Git sources: git ls-remote
	// For HTTP sources: HTTP HEAD request
	//
	// Returns nil if the source is reachable, or an error describing why
	// it could not be reached. The check has a 30-second timeout.
	//
	// If the source type is SourceTypeUnknown or SourceTypeOCI, this
	// method returns nil (no validation for fallback paths).
	ValidateReachability(ctx context.Context, recipeLocation string, templateVersion string) error
}

// IsDirectModuleSource is a convenience function that classifies the given
// recipeLocation and returns true if it represents a direct Terraform module
// source (registry, git, HTTP, S3, or GCS) rather than a wrapped/OCI recipe.
//
// This is the primary entry point for the terraform driver to determine
// which output mapping strategy to use.
func IsDirectModuleSource(recipeLocation string) bool {
	// Implementation delegates to the default resolver's Classify method.
	// Defined here as a package-level function for ergonomic usage.
	return false // placeholder
}
