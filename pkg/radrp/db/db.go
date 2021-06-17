// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Azure/radius/pkg/radrp/resources"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is an error returned when an item is not found in the database.
var ErrNotFound = errors.New("the item was not found")

// ErrConcurrency is an error returned when the item contains stale data and cannot be modified.
var ErrConcurrency = errors.New("the item has been changed")

// applicationsCollection represents the collection used to store applications in the db.
const applicationsCollection string = "applications"

// operationsCollection represents the collection used to store operations in the db.
const operationsCollection string = "operations"

// NewRadrpDB creates a new RadrpDB.
func NewRadrpDB(m *mongo.Database) RadrpDB {
	return radrpDB{
		db: m,
	}
}

//go:generate mockgen -destination=../../../mocks/mock_db.go -package=mocks github.com/Azure/radius/pkg/radrp/db RadrpDB

// RadrpDB is our database abstraction.
//
// Patch operations are an upsert operation. It creates or updates the entry. `true` will be returned for a new record.
type RadrpDB interface {
	ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]Application, error)
	GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*Application, error)
	PatchApplication(ctx context.Context, patch *ApplicationPatch) (bool, error)
	UpdateApplication(ctx context.Context, app *Application) (bool, error)
	DeleteApplicationByID(ctx context.Context, id resources.ApplicationID) error

	ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Component, error)
	GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Component, error)
	PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Component) (bool, error)
	DeleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error

	ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Deployment, error)
	GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Deployment, error)
	PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Deployment) (bool, error)
	DeleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error

	ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Scope, error)
	GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Scope, error)
	PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Scope) (bool, error)
	DeleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error

	GetOperationByID(ctx context.Context, id resources.ResourceID) (*Operation, error)
	PatchOperationByID(ctx context.Context, id resources.ResourceID, patch *Operation) (bool, error)
	DeleteOperationByID(ctx context.Context, id resources.ResourceID) error
}

type radrpDB struct {
	db *mongo.Database
}

// ListApplicationsByResourceGroup lists applications by (subscription, resource group).
func (d radrpDB) ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]Application, error) {
	items := make([]Application, 0)

	filter := bson.D{{Key: "subscriptionId", Value: id.SubscriptionID}, {Key: "resourceGroup", Value: id.ResourceGroup}}
	log.Printf("listing Applications with: %s", filter)
	col := d.db.Collection(applicationsCollection)
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return items, fmt.Errorf("error querying Applications: %w", err)
	}

	err = cursor.All(ctx, &items)
	if err != nil {
		return items, fmt.Errorf("error reading Applications: %w", err)
	}

	log.Printf("Found %d Applications with: %s", len(items), filter)
	return items, nil
}

// GetApplicationByID finds applications by fully-qualified resource id.
func (d radrpDB) GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*Application, error) {
	item := &Application{}

	filter := bson.D{{Key: "_id", Value: id.ID}}
	log.Printf("Getting %v", id)
	col := d.db.Collection(applicationsCollection)
	result := col.FindOne(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		log.Printf("%v was not found.", id)
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("error querying %v: %w", id, err)
	}

	log.Printf("Found %v", id)
	err = result.Decode(item)
	if err != nil {
		return nil, fmt.Errorf("error reading %v: %w", id, err)
	}

	return item, nil
}

func (d radrpDB) PatchApplication(ctx context.Context, patch *ApplicationPatch) (bool, error) {
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: patch.ResourceBase.ID}}
	update := bson.D{{Key: "$set", Value: patch}}

	log.Printf("Updating Application with _id: %s", patch.ResourceBase.ID)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Application: %s", err)
	}

	log.Printf("Updated Application with _id: %s - %+v", patch.ResourceBase.ID, result)
	return result.UpsertedCount > 0, nil
}

func (d radrpDB) UpdateApplication(ctx context.Context, app *Application) (bool, error) {
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: app.ResourceBase.ID}}
	update := bson.D{{Key: "$set", Value: app}}

	log.Printf("Updating Application with _id: %s", app.ResourceBase.ID)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Application: %s", err)
	}

	log.Printf("Updated Application with _id: %s - %+v", app.ResourceBase.ID, result)
	return (result.UpsertedCount > 0 || result.ModifiedCount > 0), nil
}

func (d radrpDB) DeleteApplicationByID(ctx context.Context, id resources.ApplicationID) error {
	filter := bson.D{{Key: "_id", Value: id.ID}}

	log.Printf("Deleting Application with _id: %s", id)
	col := d.db.Collection(applicationsCollection)
	result := col.FindOneAndDelete(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Application with _id: '%s': %w", id, err)
	}

	log.Printf("Deleted Application with _id: %s", id)
	return nil
}

func (d radrpDB) ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Component, error) {
	log.Printf("Listing Components with Application id: %s", id)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]Component, 0, len(application.Components))
	for _, item := range application.Components {
		items = append(items, item)
	}

	log.Printf("Found %d Component with Application id: %s", len(application.Components), id)
	return items, nil
}

func (d radrpDB) GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Component, error) {
	log.Printf("Getting Component with Application id, name, and revision: %s, %s", id, name)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	item, ok := application.Components[name]
	if !ok {
		log.Printf("Failed to find Component with Application id and name: %s, %s", id, name)
		return nil, ErrNotFound
	}

	log.Printf("Found Component with Application id, name, and revision: %s, %s, %s", id, name, item.Revision)
	return &item, nil
}

func (d radrpDB) PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Component) (bool, error) {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("components.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: patch}}}}

	log.Printf("Updating Component with Application id and name: %s, %s", id, name)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Component: %s", err)
	}

	log.Printf("Updated Component with Application id and name: %s, %s", id, name)

	return result.UpsertedCount > 1, nil
}

func (d radrpDB) DeleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("components.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key, Value: ""}}}}

	log.Printf("Deleting Component with Application id and name: %s, %s", id, name)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Component with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d radrpDB) ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Deployment, error) {
	log.Printf("Getting Deployments with Application id: %s", id)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]Deployment, 0, len(application.Deployments))
	for _, v := range application.Deployments {
		items = append(items, v)
	}

	log.Printf("Found %d Deployments with Application id: %s", len(items), id)
	return items, nil
}

func (d radrpDB) GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Deployment, error) {
	log.Printf("Getting Deployment with Application id and name: %s, %s", id, name)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	item, ok := application.Deployments[name]
	if !ok {
		log.Printf("Failed to find Deployment with Application id and name: %s, %s", id, name)
		return nil, ErrNotFound
	}

	log.Printf("Found Deployment with Application id and name: %s, %s", id, name)
	return &item, nil
}

func (d radrpDB) PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Deployment) (bool, error) {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("deployments.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: patch}}}}

	log.Printf("Updating Deployment with Application id and name: %s, %s", id, name)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Application: %s", err)
	}

	log.Printf("Updated Application with Application id and name: %s, %s - %+v", id, name, result)
	return result.UpsertedCount > 1, nil
}

func (d radrpDB) DeleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("deployments.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key}}}}

	log.Printf("Deleting Deployment with Application id and name: %s, %s, %s", id, name, update)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Deployment with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d radrpDB) ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Scope, error) {
	log.Printf("Getting Scopes with Application id: %s", id)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]Scope, 0, len(application.Scopes))
	for _, v := range application.Scopes {
		items = append(items, v)
	}

	log.Printf("Found %d Scopes with Application id: %s", len(items), id)
	return items, nil
}

func (d radrpDB) GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Scope, error) {
	log.Printf("Getting Scope with Application id and name: %s, %s", id, name)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	item, ok := application.Scopes[name]
	if !ok {
		log.Printf("Failed to find Scope with Application id and name: %s, %s", id, name)
		return nil, ErrNotFound
	}

	log.Printf("Found Scope with Application id and name: %s, %s", id, name)
	return &item, nil
}

func (d radrpDB) PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Scope) (bool, error) {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("scopes.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: patch}}}}

	log.Printf("Updating Scope with Application id and name: %s, %s", id, name)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Scope: %s", err)
	}

	log.Printf("Updated Scope with Application id and name: %s, %s - %+v", id, name, result)
	return result.UpsertedCount > 1, nil
}

func (d radrpDB) DeleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("scopes.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key, Value: ""}}}}

	log.Printf("Deleting Scope with Application id and name: %s, %s", id, name)
	col := d.db.Collection(applicationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Scope with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d radrpDB) GetOperationByID(ctx context.Context, id resources.ResourceID) (*Operation, error) {
	item := &Operation{}

	filter := bson.D{{Key: "_id", Value: id.ID}}
	log.Printf("Getting %v", id)
	col := d.db.Collection(operationsCollection)
	result := col.FindOne(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		log.Printf("%v was not found.", id)
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("error querying %v: %w", id, err)
	}

	log.Printf("Found %v", id)
	err = result.Decode(item)
	if err != nil {
		return nil, fmt.Errorf("error reading %v: %w", id, err)
	}

	return item, nil
}

func (d radrpDB) PatchOperationByID(ctx context.Context, id resources.ResourceID, patch *Operation) (bool, error) {
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: patch}}

	log.Printf("Updating Operation with _id: %s", id.ID)
	col := d.db.Collection(operationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Operation: %s", err)
	}

	log.Printf("Updated Operation with _id: %s - %+v", id.ID, result)
	return result.UpsertedCount > 1, nil
}

func (d radrpDB) DeleteOperationByID(ctx context.Context, id resources.ResourceID) error {
	filter := bson.D{{Key: "_id", Value: id.ID}}

	log.Printf("Deleting Operation with _id: %s", id)
	col := d.db.Collection(operationsCollection)
	result := col.FindOneAndDelete(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Operation with _id: '%s': %w", id, err)
	}

	log.Printf("Deleted Operation with _id: %s", id)
	return nil
}
