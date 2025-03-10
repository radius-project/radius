package database

import "context"

// Snapshotter defines a method for taking a snapshot of a data store.
type Snapshotter interface {
	Snapshot(ctx context.Context) ([]byte, error)
}

// Restorer defines a method for restoring a data store from a snapshot.
type Restorer interface {
	Restore(ctx context.Context, snapshot []byte) error
}
