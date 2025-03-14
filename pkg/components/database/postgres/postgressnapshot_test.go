package postgres

// import (
// 	"context"
// 	"encoding/json"
// 	"testing"
// 	"time"

// 	"github.com/pashagolub/pgxmock"
// 	"github.com/stretchr/testify/assert"
// )

// func TestPostgresClient_Snapshot(t *testing.T) {
// 	mock, err := pgxmock.NewConn()
// 	assert.NoError(t, err)
// 	defer mock.Close(context.Background())

// 	client := NewPostgresClient(mock)

// 	ctx := context.Background()
// 	databases := []string{"ucp", "applications_rp"}

// 	for _, db := range databases {
// 		mock.ExpectExec("SET search_path TO " + db).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
// 		mock.ExpectQuery("SELECT id, original_id, resource_type, root_scope, routing_scope, etag, created_at, resource_data FROM resources").
// 			WillReturnRows(pgxmock.NewRows([]string{"id", "original_id", "resource_type", "root_scope", "routing_scope", "etag", "created_at", "resource_data"}).
// 				AddRow("1", "1", "type1", "scope1", "route1", "etag1", time.Now(), `{"key":"value"}`))
// 	}

// 	snapshot, err := client.Snapshot(ctx)
// 	assert.NoError(t, err)

// 	var allSnapshots []map[string]interface{}
// 	err = json.Unmarshal(snapshot, &allSnapshots)
// 	assert.NoError(t, err)
// 	assert.Len(t, allSnapshots, 2)

// 	err = mock.ExpectationsWereMet()
// 	assert.NoError(t, err)
// }

// func TestPostgresClient_Restore(t *testing.T) {
// 	mock, err := pgxmock.NewConn()
// 	assert.NoError(t, err)
// 	defer mock.Close(context.Background())

// 	client := NewPostgresClient(mock)

// 	ctx := context.Background()
// 	databases := []string{"ucp", "applications_rp"}

// 	snapshotData := []map[string]interface{}{
// 		{
// 			"database": "ucp",
// 			"records": []resourceRecord{
// 				{
// 					ID:           "1",
// 					OriginalID:   "1",
// 					ResourceType: "type1",
// 					RootScope:    "scope1",
// 					RoutingScope: "route1",
// 					ETag:         "etag1",
// 					CreatedAt:    time.Now(),
// 					ResourceData: json.RawMessage(`{"key":"value"}`),
// 				},
// 			},
// 		},
// 		{
// 			"database": "applications_rp",
// 			"records": []resourceRecord{
// 				{
// 					ID:           "2",
// 					OriginalID:   "2",
// 					ResourceType: "type2",
// 					RootScope:    "scope2",
// 					RoutingScope: "route2",
// 					ETag:         "etag2",
// 					CreatedAt:    time.Now(),
// 					ResourceData: json.RawMessage(`{"key":"value"}`),
// 				},
// 			},
// 		},
// 	}

// 	snapshot, err := json.Marshal(snapshotData)
// 	assert.NoError(t, err)

// 	for _, db := range databases {
// 		mock.ExpectExec("SET search_path TO " + db).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
// 		mock.ExpectBegin()
// 		mock.ExpectExec("DELETE FROM resources").WillReturnResult(pgxmock.NewResult("DELETE", 1))
// 		mock.ExpectExec("INSERT INTO resources").
// 			WithArgs("1", "1", "type1", "scope1", "route1", "etag1", pgxmock.AnyArg(), `{"key":"value"}`).
// 			WillReturnResult(pgxmock.NewResult("INSERT", 1))
// 		mock.ExpectCommit()
// 	}

// 	err = client.Restore(ctx, snapshot)
// 	assert.NoError(t, err)

// 	err = mock.ExpectationsWereMet()
// 	assert.NoError(t, err)
// }
