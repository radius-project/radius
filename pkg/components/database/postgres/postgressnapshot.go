package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// resourceRecord represents a row in the resources table for snapshot/restore purposes.
type resourceRecord struct {
	ID           string          `json:"id"`
	OriginalID   string          `json:"original_id"`
	ResourceType string          `json:"resource_type"`
	RootScope    string          `json:"root_scope"`
	RoutingScope string          `json:"routing_scope"`
	ETag         string          `json:"etag"`
	CreatedAt    time.Time       `json:"created_at"`
	ResourceData json.RawMessage `json:"resource_data"`
}

// Snapshot implements the Snapshotter interface for PostgresClient.
// It retrieves all rows from the "resources" table and returns an indented JSON array.
func (p *PostgresClient) Snapshot(ctx context.Context) ([]byte, error) {
	databases := []string{"ucp", "applications_rp"}
	var allSnapshots []map[string]interface{}

	for _, db := range databases {
		if err := p.switchDatabase(ctx, db); err != nil {
			return nil, fmt.Errorf("failed to switch to database %s: %w", db, err)
		}

		sql := `
SELECT id, original_id, resource_type, root_scope, routing_scope, etag, created_at, resource_data
FROM resources;`

		rows, err := p.api.Query(ctx, sql)
		if err != nil {
			return nil, fmt.Errorf("failed to execute snapshot query: %w", err)
		}
		defer rows.Close()

		var records []resourceRecord
		for rows.Next() {
			var rec resourceRecord
			if err := rows.Scan(&rec.ID, &rec.OriginalID, &rec.ResourceType, &rec.RootScope, &rec.RoutingScope, &rec.ETag, &rec.CreatedAt, &rec.ResourceData); err != nil {
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}
			records = append(records, rec)
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error reading rows: %w", err)
		}

		allSnapshots = append(allSnapshots, map[string]interface{}{
			"database": db,
			"records":  records,
		})
	}

	snapshotData, err := json.MarshalIndent(allSnapshots, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	return snapshotData, nil
}

// Restore implements the Restorer interface for PostgresClient.
// It unmarshals the snapshot data and restores the "resources" table.
// This implementation deletes all current rows and reinserts the rows from the snapshot.
func (p *PostgresClient) Restore(ctx context.Context, snapshot []byte) error {
	var allSnapshots []map[string]interface{}
	if err := json.Unmarshal(snapshot, &allSnapshots); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot data: %w", err)
	}

	for _, dbSnapshot := range allSnapshots {
		db, ok := dbSnapshot["database"].(string)
		if !ok {
			return fmt.Errorf("invalid snapshot format: missing database name")
		}

		records, ok := dbSnapshot["records"].([]resourceRecord)
		if !ok {
			return fmt.Errorf("invalid snapshot format: missing records")
		}

		if err := p.switchDatabase(ctx, db); err != nil {
			return fmt.Errorf("failed to switch to database %s: %w", db, err)
		}

		// Begin a transaction.
		tx, err := p.api.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		// Delete current data.
		_, err = tx.Exec(ctx, "DELETE FROM resources;")
		if err != nil {
			return fmt.Errorf("failed to clear resources table: %w", err)
		}

		// Prepare an INSERT for all columns.
		stmt := `
INSERT INTO resources (id, original_id, resource_type, root_scope, routing_scope, etag, created_at, resource_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`
		for _, rec := range records {
			_, err = tx.Exec(ctx, stmt,
				rec.ID,
				rec.OriginalID,
				rec.ResourceType,
				rec.RootScope,
				rec.RoutingScope,
				rec.ETag,
				rec.CreatedAt,
				rec.ResourceData)
			if err != nil {
				return fmt.Errorf("failed to insert record (id: %s): %w", rec.ID, err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit restore transaction: %w", err)
		}
	}

	return nil
}

// switchDatabase switches the current database context.
func (p *PostgresClient) switchDatabase(ctx context.Context, dbName string) error {
	_, err := p.api.Exec(ctx, fmt.Sprintf("SET search_path TO %s;", dbName))
	return err
}
