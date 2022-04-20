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

	GetOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	DeleteOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	SaveOptions interface {
		ApplySaveOption(StoreConfig) StoreConfig

		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}
)

// Store Config
type StoreConfig struct {
	PaginationToken string
	ETag            ETag
}

// Query Options
type queryOptions struct {
	fn func(StoreConfig) StoreConfig
}

func (q *queryOptions) ApplyQueryOption(cfg StoreConfig) StoreConfig {
	return q.fn(cfg)
}

func (q queryOptions) private() {}

func WithPaginationToken(token string) QueryOptions {
	return &queryOptions{
		fn: func(cfg StoreConfig) StoreConfig {
			cfg.PaginationToken = token
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
