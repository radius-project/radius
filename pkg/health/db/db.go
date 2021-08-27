// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"errors"

	"go.mongodb.org/mongo-driver/mongo"
)

// ErrNotFound is an error returned when an item is not found in the database.
var ErrNotFound = errors.New("the item was not found")

// ErrConcurrency is an error returned when the item contains stale data and cannot be modified.
var ErrConcurrency = errors.New("the item has been changed")

// NewRadHealthDB creates a new HealthDB.
func NewRadHealthDB(m *mongo.Database) RadHealthDB {
	return radHealthDB{
		db: m,
	}
}

//go:generate mockgen -destination=./mock_db.go -package=db -self_package github.com/Azure/radius/pkg/health/db github.com/Azure/radius/pkg/health/db RadHealthDB

// RadHealthDB is our database abstraction.
type RadHealthDB interface {
}

type radHealthDB struct {
	db *mongo.Database
}
