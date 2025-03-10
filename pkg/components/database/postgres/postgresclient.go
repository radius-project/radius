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

package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseutil"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/util/etag"
)

//go:generate mockgen -typed -destination=./mock_postgresapi.go -package=postgres -self_package github.com/radius-project/radius/pkg/components/database/postgres github.com/radius-project/radius/pkg/components/database/postgres PostgresAPI

// PostgresAPI defines the API surface from pgx that we use. This is used to allow for easier testing.
//
// Keep these definitions in sync with pgxpool.Pool and pgx.Conn.
type PostgresAPI interface {
	// Exec executes a query without returning any rows.
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	// QueryRow executes a query that is expected to return at most one row.
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	// Query executes a query that returns rows.
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	// Begin starts a new transaction.
	Begin(ctx context.Context) (pgx.Tx, error)
}

// NewPostgresClient creates a new PostgresClient.
func NewPostgresClient(api PostgresAPI) *PostgresClient {
	return &PostgresClient{api: api}
}

var _ database.Client = (*PostgresClient)(nil)

// PostgresClient is a database client that uses Postgres as the backend.
type PostgresClient struct {
	api PostgresAPI
}

// Delete implements database.Client.
func (p *PostgresClient) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
	if ctx == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}

	parsed, err := resources.Parse(id)
	if err != nil {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return &database.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	converted, err := databaseutil.ConvertScopeIDToResourceID(parsed)
	if err != nil {
		return err
	}

	config := database.NewDeleteConfig(options...)
	var etag *string
	if config.ETag != "" {
		etag = &config.ETag
	}

	// We need different SQL for the case where an etag is provided vs not provided.
	//
	// The key behavior difference is that if an etag is provided, should report failure differently.
	sql := `
WITH deleted AS (
	DELETE FROM resources
	WHERE id = $1
	RETURNING id
)
SELECT
CASE
	WHEN EXISTS (SELECT 1 FROM deleted) THEN 'Success'
	WHEN EXISTS (SELECT 1 FROM resources WHERE id = $1) THEN 'ErrConcurrency'
	ELSE 'ErrNotFound'
END AS result;`

	args := []any{databaseutil.NormalizePart(converted.String())}

	if config.ETag != "" {
		// NOTE: we want to report ErrConcurrency for all failure cases here. This is what the tests do.
		sql = `
WITH deleted AS (
	DELETE FROM resources
	WHERE id = $1 AND etag = $2
	RETURNING id
)
SELECT
CASE
	WHEN EXISTS (SELECT 1 FROM deleted) THEN 'Success'
	WHEN EXISTS (SELECT 1 FROM resources WHERE id = $1) THEN 'ErrConcurrency'
	ELSE 'ErrConcurrency'
END AS result;`

		args = []any{databaseutil.NormalizePart(converted.String()), etag}
	}

	result := ""
	err = p.api.QueryRow(ctx, sql, args...).Scan(&result)
	if err != nil {
		return err
	} else if result == "ErrNotFound" {
		return &database.ErrNotFound{ID: id}
	} else if result == "ErrConcurrency" {
		return &database.ErrConcurrency{}
	}

	return nil
}

// Get implements database.Client.
func (p *PostgresClient) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if ctx == nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}

	parsed, err := resources.Parse(id)
	if err != nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	converted, err := databaseutil.ConvertScopeIDToResourceID(parsed)
	if err != nil {
		return nil, err
	}

	obj := database.Object{}
	err = p.api.QueryRow(
		ctx,
		"SELECT original_id, etag, resource_data FROM resources WHERE id = $1",
		databaseutil.NormalizePart(converted.String())).Scan(&obj.ID, &obj.ETag, &obj.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &database.ErrNotFound{ID: id}
	} else if err != nil {
		return nil, err
	}

	return &obj, nil
}

// Query implements database.Client.
func (p *PostgresClient) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}

	err := query.Validate()
	if err != nil {
		return nil, &database.ErrInvalid{Message: fmt.Sprintf("invalid argument. Query is invalid: %s", err.Error())}
	}

	config := database.NewQueryConfig(options...)

	// For a scope query, we need to perform the same normalization as we do for other operations on scopes.
	resourceType := databaseutil.NormalizePart(query.ResourceType)
	if query.IsScopeQuery {
		var err error
		resourceType, err = databaseutil.ConvertScopeTypeToResourceType(query.ResourceType)
		if err != nil {
			return nil, err
		}

		resourceType = databaseutil.NormalizePart(resourceType)
	}

	var routingScopePrefixFilter *string
	if query.RoutingScopePrefix != "" {
		routingScopePrefixFilter = to.Ptr(databaseutil.NormalizePart(query.RoutingScopePrefix))
	}

	var timestampFilter *string
	if config.PaginationToken != "" {
		ts, err := p.parsePaginationToken(config.PaginationToken)
		if err != nil {
			return nil, &database.ErrInvalid{Message: "invalid argument. 'query.PaginationToken' is invalid."}
		}
		timestampFilter = &ts
	}

	var limitFilter *int
	if config.MaxQueryItemCount > 0 {
		limitFilter = &config.MaxQueryItemCount
	}

	// For a scope query, we need to perform the same normalization as we do for other operations on scopes.
	if query.IsScopeQuery {
		var err error
		query.ResourceType, err = databaseutil.ConvertScopeTypeToResourceType(query.ResourceType)
		if err != nil {
			return nil, err
		}
	}

	// NOTE: building SQL by concatenating strings is hard to do safely and should be avoided.
	// If you need to work on this code MAKE SURE you use SQL parameters
	// for any user input.
	sql := `
SELECT original_id, etag, resource_data, created_at 
FROM resources
WHERE ((root_scope = $1) OR ($2 AND (root_scope LIKE $1 || '%'))) AND 
	resource_type = $3 AND 
	((routing_scope LIKE $4 || '%') OR $4 IS NULL) AND 
	(created_at > $5::TIMESTAMP OR $5 IS NULL)
ORDER BY created_at ASC
LIMIT $6`

	args := []any{
		// If ScopeRecursive is false, the RootScope must match exactly.
		// If ScopeRecursive is true, the RootScope must be a prefix of the stored RootScope.
		databaseutil.NormalizePart(query.RootScope),
		query.ScopeRecursive,
		resourceType,
		routingScopePrefixFilter, // RoutingScopePrefix is optional and always treated as as prefix.
		timestampFilter,          // Optional for pagination.
		limitFilter,              // NOTE: Postgres allows LIMIT to be set with a NULL value to mean no limit.
	}

	rows, err := p.api.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Capture the last timestamp so we can use it for pagination.
	var timestamp *time.Time

	result := database.ObjectQueryResult{}
	for rows.Next() {
		obj := database.Object{}
		err := rows.Scan(&obj.ID, &obj.ETag, &obj.Data, &timestamp)
		if err != nil {
			return nil, err
		}

		// We could improve this by moving the filter logic to the SQL query.
		//
		// The problem is that the current filter logic is not well documented or tested, and
		// we want to stay compatible with the existing implementation for now.
		match, err := obj.MatchesFilters(query.Filters)
		if err != nil {
			return nil, err
		} else if !match {
			continue
		}

		result.Items = append(result.Items, obj)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	if len(result.Items) < config.MaxQueryItemCount && config.MaxQueryItemCount > 0 {
		// No more rows, so no need for pagination.
		return &result, nil
	}

	if timestamp != nil {
		// Will be empty if there were no rows.
		token, err := p.createPaginationToken(*timestamp)
		if err != nil {
			return nil, err
		}
		result.PaginationToken = token
	}

	return &result, nil
}

// Save implements database.Client.
func (p *PostgresClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if ctx == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &database.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	parsed, err := resources.Parse(obj.ID)
	if err != nil {
		return &database.ErrInvalid{Message: "invalid argument. 'obj.ID' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return &database.ErrInvalid{Message: "invalid argument. 'obj.ID' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return &database.ErrInvalid{Message: "invalid argument. 'obj.ID' must refer to a named resource, not a collection"}
	}

	converted, err := databaseutil.ConvertScopeIDToResourceID(parsed)
	if err != nil {
		return err
	}

	config := database.NewSaveConfig(options...)

	// Compute ETag for the current state of the object.
	raw, err := json.Marshal(obj.Data)
	if err != nil {
		return err
	}

	obj.ETag = etag.New(raw)

	// We need different SQL for the case where an etag is provided vs not provided.
	//
	// The key behavior difference is that if an etag is provided, we should not perform inserts, only updates.

	// This is the more complex query that handles "upserts". It does not process etags.
	sql := `
WITH updated AS (
	INSERT INTO resources (id, original_id, resource_type, root_scope, routing_scope, etag, resource_data)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (id) 
	DO UPDATE SET resource_data = $7
	RETURNING id
)
SELECT
CASE
	WHEN EXISTS (SELECT 1 FROM updated) THEN 'Success'
	WHEN EXISTS (SELECT 1 FROM resources WHERE id = $1) THEN 'ErrConcurrency'
	ELSE 'ErrNotFound'
END AS result;`

	args := []any{
		databaseutil.NormalizePart(converted.String()),
		obj.ID, // MUST NOT BE NORMALIZED. Preserve the original casing and format.
		databaseutil.NormalizePart(converted.Type()),
		databaseutil.NormalizePart(converted.RootScope()),
		databaseutil.NormalizePart(converted.RoutingScope()),
		obj.ETag,
		obj.Data,
	}

	if config.ETag != "" {
		// This is the simpler query that only performs updates. It requires an etag.
		// NOTE: we want to report ErrConcurrency for all failure cases here. This is what the tests do.
		sql = `
WITH updated AS (
	UPDATE resources SET resource_data = $2
	WHERE id = $1 AND etag = $3
	RETURNING id
)
SELECT
CASE
	WHEN EXISTS (SELECT 1 FROM updated) THEN 'Success'
	WHEN EXISTS (SELECT 1 FROM resources WHERE id = $1) THEN 'ErrConcurrency'
	ELSE 'ErrConcurrency'
END AS result;`

		args = []any{databaseutil.NormalizePart(converted.String()), obj.Data, config.ETag}
	}

	result := ""
	err = p.api.QueryRow(ctx, sql, args...).Scan(&result)
	if err != nil {
		return err
	} else if result == "ErrNotFound" {
		return &database.ErrNotFound{ID: obj.ID}
	} else if result == "ErrConcurrency" {
		return &database.ErrConcurrency{}
	}

	return nil
}

// createPaginationToken converts a timestamp to a base64 encoded string.
//
// We use ISO8601/RFC3339 format which postgres understands and can be used for comparison.
// We also add microseconds to the timestamp to ensure uniqueness.
func (p *PostgresClient) createPaginationToken(timestamp time.Time) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(timestamp.UTC().Format(time.RFC3339Nano))), nil
}

// parsePaginationToken converts a base64 encoded string to a timestamp.
func (p *PostgresClient) parsePaginationToken(token string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", err
	}

	// Roundtripping to ensure that we understand the data.
	parsed, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		return "", err
	}

	return parsed.UTC().Format(time.RFC3339Nano), nil
}
