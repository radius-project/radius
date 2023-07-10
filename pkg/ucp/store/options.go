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

package store

type (
	// QueryOptions applies an option to Query().
	QueryOptions interface {
		ApplyQueryOption(StoreConfig) StoreConfig

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// GetOptions applies an option to Get().
	GetOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// DeleteOptions applies an option to Delete().
	DeleteOptions interface {
		ApplyDeleteOption(StoreConfig) StoreConfig

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// SaveOptions applies an option to Save().
	SaveOptions interface {
		ApplySaveOption(StoreConfig) StoreConfig

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// MutatingOptions applies an option to Delete() or Save().
	MutatingOptions interface {
		SaveOptions
		DeleteOptions
	}
)

// Store Config represents the configurations of storageclient APIs.
type StoreConfig struct {
	// PaginationToken represents pagination token such as continuation token.
	PaginationToken string

	// MaxQueryItemCount represents max items in query result.
	MaxQueryItemCount int

	// ETag represents the entity tag for optimistic consistency control.
	ETag ETag
}

// Query Options
type queryOptions struct {
	fn func(StoreConfig) StoreConfig
}

// # Function Explanation
//
// ApplyQueryOption applies a function to the StoreConfig to modify it.
func (q *queryOptions) ApplyQueryOption(cfg StoreConfig) StoreConfig {
	return q.fn(cfg)
}

func (q queryOptions) private() {}

// # Function Explanation
//
// WithPaginationToken sets pagination token for Query().
func WithPaginationToken(token string) QueryOptions {
	return &queryOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.PaginationToken = token
			return cfg
		},
	}
}

// # Function Explanation
//
// WithMaxQueryItemCount creates a QueryOptions instance that sets the maximum number of items in query result.
func WithMaxQueryItemCount(maxcnt int) QueryOptions {
	return &queryOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.MaxQueryItemCount = maxcnt
			return cfg
		},
	}
}

// MutatingOptions
type mutatingOptions struct {
	fn func(StoreConfig) StoreConfig
}

var _ DeleteOptions = (*mutatingOptions)(nil)
var _ SaveOptions = (*mutatingOptions)(nil)

// # Function Explanation
//
// ApplyDeleteOption applies the delete option to the given StoreConfig and returns the modified StoreConfig.
func (s *mutatingOptions) ApplyDeleteOption(cfg StoreConfig) StoreConfig {
	return s.fn(cfg)
}

// # Function Explanation
//
// ApplySaveOption applies the save option to the given StoreConfig and returns the modified StoreConfig.
func (s *mutatingOptions) ApplySaveOption(cfg StoreConfig) StoreConfig {
	return s.fn(cfg)
}

func (s mutatingOptions) private() {}

// SaveOptions
type saveOptions struct {
	fn func(StoreConfig) StoreConfig
}

var _ SaveOptions = (*saveOptions)(nil)

// # Function Explanation
//
// ApplySaveOption applies a save option to a StoreConfig.
func (s *saveOptions) ApplySaveOption(cfg StoreConfig) StoreConfig {
	return s.fn(cfg)
}

func (s saveOptions) private() {}

// # Function Explanation
//
// WithETag sets the ETag field in the StoreConfig struct.
func WithETag(etag ETag) MutatingOptions {
	return &mutatingOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.ETag = etag
			return cfg
		},
	}
}

// # Function Explanation
//
// NewQueryConfig applies a set of QueryOptions to a StoreConfig and returns the modified StoreConfig for Query().
func NewQueryConfig(opts ...QueryOptions) StoreConfig {
	cfg := StoreConfig{}
	for _, opt := range opts {
		cfg = opt.ApplyQueryOption(cfg)
	}
	return cfg
}

// # Function Explanation
//
// NewDeleteConfig applies the given DeleteOptions to a StoreConfig and returns the resulting StoreConfig for Delete().
func NewDeleteConfig(opts ...DeleteOptions) StoreConfig {
	cfg := StoreConfig{}
	for _, opt := range opts {
		cfg = opt.ApplyDeleteOption(cfg)
	}
	return cfg
}

// # Function Explanation
//
// NewSaveConfig applies a set of SaveOptions to a StoreConfig and returns the modified StoreConfig for Save().
func NewSaveConfig(opts ...SaveOptions) StoreConfig {
	cfg := StoreConfig{}
	for _, opt := range opts {
		cfg = opt.ApplySaveOption(cfg)
	}
	return cfg
}
