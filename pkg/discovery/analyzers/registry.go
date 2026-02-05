package analyzers

import (
	"context"
	"fmt"
	"sync"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
)

// Registry manages available analyzers and provides language detection.
type Registry struct {
	mu        sync.RWMutex
	analyzers map[string]Analyzer
}

// NewRegistry creates a new analyzer registry.
func NewRegistry() *Registry {
	return &Registry{
		analyzers: make(map[string]Analyzer),
	}
}

// Register adds an analyzer to the registry.
func (r *Registry) Register(analyzer Analyzer) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := analyzer.Name()
	if _, exists := r.analyzers[name]; exists {
		return fmt.Errorf("analyzer %q already registered", name)
	}
	r.analyzers[name] = analyzer
	return nil
}

// Get returns an analyzer by name.
func (r *Registry) Get(name string) (Analyzer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	analyzer, ok := r.analyzers[name]
	return analyzer, ok
}

// All returns all registered analyzers.
func (r *Registry) All() []Analyzer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Analyzer, 0, len(r.analyzers))
	for _, a := range r.analyzers {
		result = append(result, a)
	}
	return result
}

// ForLanguage returns analyzers that support the given language.
func (r *Registry) ForLanguage(lang dtypes.Language) []Analyzer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Analyzer, 0)
	for _, a := range r.analyzers {
		for _, l := range a.Languages() {
			if l == lang {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// DetectAndAnalyze probes the project path to find applicable analyzers,
// then runs them to collect dependencies.
func (r *Registry) DetectAndAnalyze(ctx context.Context, projectPath string, opts AnalyzeOptions) ([]*AnalyzeResult, error) {
	r.mu.RLock()
	analyzers := make([]Analyzer, 0, len(r.analyzers))
	for _, a := range r.analyzers {
		analyzers = append(analyzers, a)
	}
	r.mu.RUnlock()

	var results []*AnalyzeResult

	for _, analyzer := range analyzers {
		canAnalyze, err := analyzer.CanAnalyze(ctx, projectPath)
		if err != nil {
			// Log warning but continue with other analyzers
			continue
		}
		if !canAnalyze {
			continue
		}

		result, err := analyzer.Analyze(ctx, projectPath, opts)
		if err != nil {
			// Continue with other analyzers even if one fails
			continue
		}

		if result != nil && (len(result.Dependencies) > 0 || len(result.Services) > 0) {
			results = append(results, result)
		}
	}

	return results, nil
}

// DefaultRegistry is the global analyzer registry.
var DefaultRegistry = NewRegistry()

// Register adds an analyzer to the default registry.
func Register(analyzer Analyzer) error {
	return DefaultRegistry.Register(analyzer)
}
