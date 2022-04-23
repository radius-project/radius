// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

func (q *queryOptions) ApplyQueryOption(cfg StoreConfig) StoreConfig {
	return q.fn(cfg)
}

func (q queryOptions) private() {}

// WithPaginationToken sets pagination token for Query().
func WithPaginationToken(token string) QueryOptions {
	return &queryOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.PaginationToken = token
			return cfg
		},
	}
}

// WithMaxQueryItemCount sets max items in query result.
func WithMaxQueryItemCount(maxcnt int) QueryOptions {
	return &queryOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.MaxQueryItemCount = maxcnt
			return cfg
		},
	}
}

// SaveOptions
type saveOptions struct {
	fn func(StoreConfig) StoreConfig
}

func (s *saveOptions) ApplySaveOption(cfg StoreConfig) StoreConfig {
	return s.fn(cfg)
}

func (s saveOptions) private() {}

// WithETag sets the etag for Save().
func WithETag(etag ETag) SaveOptions {
	return &saveOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.ETag = etag
			return cfg
		},
	}
}

// NewQueryConfig returns new store config for Query().
func NewQueryConfig(opts ...QueryOptions) StoreConfig {
	cfg := StoreConfig{}
	for _, opt := range opts {
		cfg = opt.ApplyQueryOption(cfg)
	}
	return cfg
}

// NewSaveConfig returns new store config for Save().
func NewSaveConfig(opts ...SaveOptions) StoreConfig {
	cfg := StoreConfig{}
	for _, opt := range opts {
		cfg = opt.ApplySaveOption(cfg)
	}
	return cfg
}
