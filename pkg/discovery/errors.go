package discovery

import (
	"errors"
	"fmt"
)

// Common sentinel errors for the discovery package.
var (
	// ErrProjectNotFound indicates the specified project path does not exist.
	ErrProjectNotFound = errors.New("project path not found")

	// ErrNoLanguageDetected indicates no supported programming language was detected.
	ErrNoLanguageDetected = errors.New("no supported programming language detected")

	// ErrNoDependenciesFound indicates no infrastructure dependencies were detected.
	ErrNoDependenciesFound = errors.New("no infrastructure dependencies found")

	// ErrNoServicesFound indicates no deployable services were detected.
	ErrNoServicesFound = errors.New("no deployable services found")

	// ErrDiscoveryNotFound indicates discovery results are required but not found.
	ErrDiscoveryNotFound = errors.New("discovery.md not found; run 'rad app discover' first")

	// ErrInvalidDiscoveryResult indicates the discovery result is malformed.
	ErrInvalidDiscoveryResult = errors.New("invalid discovery result")

	// ErrOutputExists indicates the output file already exists.
	ErrOutputExists = errors.New("output file already exists; use --force to overwrite")

	// ErrBicepValidation indicates the generated Bicep failed validation.
	ErrBicepValidation = errors.New("generated Bicep failed validation")

	// ErrRecipeSourceUnavailable indicates a recipe source could not be reached.
	ErrRecipeSourceUnavailable = errors.New("recipe source unavailable")

	// ErrNoRecipeMatch indicates no recipe matched the dependency.
	ErrNoRecipeMatch = errors.New("no matching recipe found")

	// ErrResourceTypeCatalogNotFound indicates the Resource Type catalog is missing.
	ErrResourceTypeCatalogNotFound = errors.New("resource type catalog not found")

	// ErrLibraryCatalogNotFound indicates the library catalog is missing.
	ErrLibraryCatalogNotFound = errors.New("library catalog not found")
)

// AnalyzerError represents an error from a language analyzer.
type AnalyzerError struct {
	Language Language
	FilePath string
	Err      error
}

func (e *AnalyzerError) Error() string {
	return fmt.Sprintf("analyzer error for %s in %s: %v", e.Language, e.FilePath, e.Err)
}

func (e *AnalyzerError) Unwrap() error {
	return e.Err
}

// ParseError represents an error parsing a manifest file.
type ParseError struct {
	File   string
	Line   int
	Column int
	Err    error
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("parse error in %s at line %d: %v", e.File, e.Line, e.Err)
	}
	return fmt.Sprintf("parse error in %s: %v", e.File, e.Err)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// ValidationError represents a validation error in generated output.
type ValidationError struct {
	File    string
	Line    int
	Column  int
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s - %s", e.File, e.Line, e.Column, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s - %s", e.File, e.Code, e.Message)
}

// RecipeSourceError represents an error accessing a recipe source.
type RecipeSourceError struct {
	SourceType RecipeSourceType
	Location   string
	Err        error
}

func (e *RecipeSourceError) Error() string {
	return fmt.Sprintf("recipe source error for %s at %s: %v", e.SourceType, e.Location, e.Err)
}

func (e *RecipeSourceError) Unwrap() error {
	return e.Err
}

// MultiError collects multiple errors during discovery.
type MultiError struct {
	Errors []error
}

func (e *MultiError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred during discovery", len(e.Errors))
}

// Add appends an error to the collection.
func (e *MultiError) Add(err error) {
	if err != nil {
		e.Errors = append(e.Errors, err)
	}
}

// HasErrors returns true if any errors were collected.
func (e *MultiError) HasErrors() bool {
	return len(e.Errors) > 0
}

// Unwrap returns the first error for errors.Is/As compatibility.
func (e *MultiError) Unwrap() error {
	if len(e.Errors) > 0 {
		return e.Errors[0]
	}
	return nil
}

// NewMultiError creates a new MultiError.
func NewMultiError() *MultiError {
	return &MultiError{Errors: make([]error, 0)}
}

// IsPartialSuccess returns true if discovery completed with some warnings.
func IsPartialSuccess(err error) bool {
	var me *MultiError
	return errors.As(err, &me)
}
