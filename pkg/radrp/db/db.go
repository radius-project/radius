// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radlogger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is an error returned when an item is not found in the database.
var ErrNotFound = errors.New("the item was not found")

// ErrConflict is an error returned when the application has existing child resources.
var ErrConflict = errors.New("the resource has existing child resources")

// applicationsV3Collection represents the collection used to store applications in the db for app model v3.
const applicationsV3Collection string = "applicationsv3"

// resourcesCollection represents the collection used to store resources in the db.
const resourcesCollection string = "resources"

// operationsCollection represents the collection used to store operations in the db.
const operationsCollection string = "operations"

// NewRadrpDB creates a new RadrpDB.
func NewRadrpDB(m *mongo.Database) RadrpDB {
	return radrpDB{
		db: m,
	}
}

//go:generate mockgen -destination=./mock_db.go -package=db -self_package github.com/Azure/radius/pkg/radrp/db github.com/Azure/radius/pkg/radrp/db RadrpDB

// RadrpDB is our database abstraction.
//
// Patch operations are an upsert operation. It creates or updates the entry. `true` will be returned for a new record.
type RadrpDB interface {
	GetOperationByID(ctx context.Context, id azresources.ResourceID) (*Operation, error)
	PatchOperationByID(ctx context.Context, id azresources.ResourceID, patch *Operation) (bool, error)
	DeleteOperationByID(ctx context.Context, id azresources.ResourceID) error

	ListV3Applications(ctx context.Context, id azresources.ResourceID) ([]ApplicationResource, error)
	GetV3Application(ctx context.Context, id azresources.ResourceID) (ApplicationResource, error)
	UpdateV3ApplicationDefinition(ctx context.Context, application ApplicationResource) (bool, error)
	DeleteV3Application(ctx context.Context, id azresources.ResourceID) error

	ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error)
	ListV3Resources(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error)
	GetV3Resource(ctx context.Context, id azresources.ResourceID) (RadiusResource, error)
	UpdateV3ResourceDefinition(ctx context.Context, id azresources.ResourceID, resource RadiusResource) (bool, error)
	UpdateV3ResourceStatus(ctx context.Context, id azresources.ResourceID, resource RadiusResource) error
	DeleteV3Resource(ctx context.Context, id azresources.ResourceID) error
}

type radrpDB struct {
	db *mongo.Database
}

func (d radrpDB) GetOperationByID(ctx context.Context, id azresources.ResourceID) (*Operation, error) {
	item := &Operation{}

	filter := bson.D{{Key: "_id", Value: id.ID}}
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldOperationID, id)
	logger.Info(fmt.Sprintf("Getting operation from DB with operation filter: %s", filter))
	col := d.db.Collection(operationsCollection)
	result := col.FindOne(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		logger.Info("operation was not found.")
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("error querying %v: %w", id, err)
	}

	logger.Info("Found operation in DB")
	err = result.Decode(item)
	if err != nil {
		return nil, fmt.Errorf("error reading %v: %w", id, err)
	}

	return item, nil
}

func (d radrpDB) PatchOperationByID(ctx context.Context, id azresources.ResourceID, patch *Operation) (bool, error) {
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldOperationID, id)
	update := bson.D{{Key: "$set", Value: patch}}

	logger.Info(fmt.Sprintf("Updating operation in DB with operation filter: %s", filter))
	col := d.db.Collection(operationsCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Operation: %s", err)
	}

	logger.Info(fmt.Sprintf("Updated operation in DB - %+v", result))
	return result.UpsertedCount > 1, nil
}

func (d radrpDB) DeleteOperationByID(ctx context.Context, id azresources.ResourceID) error {
	filter := bson.D{{Key: "_id", Value: id.ID}}
	logger := radlogger.GetLogger(ctx).WithValues(
		radlogger.LogFieldOperationID, id)
	logger.Info(fmt.Sprintf("Deleting operation from DB with operation filter: %s", filter))
	col := d.db.Collection(operationsCollection)
	result := col.FindOneAndDelete(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Operation with _id: '%s': %w", id, err)
	}

	logger.Info("Deleted operation from DB")
	return nil
}

func (d radrpDB) ListV3Applications(ctx context.Context, id azresources.ResourceID) ([]ApplicationResource, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id.ID)
	items := make([]ApplicationResource, 0)

	filter := bson.D{{Key: "subscriptionId", Value: id.SubscriptionID}, {Key: "resourceGroup", Value: id.ResourceGroup}}

	logger.Info(fmt.Sprintf("Listing applications from DB with operation filter: %s", filter))
	col := d.db.Collection(applicationsV3Collection)
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return items, fmt.Errorf("error querying Applications: %w", err)
	}

	err = cursor.All(ctx, &items)
	if err != nil {
		return items, fmt.Errorf("error reading Applications: %w", err)
	}
	logger.Info(fmt.Sprintf("Found %d Applications", len(items)))

	return items, nil
}

func (d radrpDB) GetV3Application(ctx context.Context, id azresources.ResourceID) (ApplicationResource, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id.ID)
	item := ApplicationResource{}

	filter := bson.D{{Key: "_id", Value: id.ID}}

	logger.Info(fmt.Sprintf("Getting application from DB with operation filter: %s", filter))
	col := d.db.Collection(applicationsV3Collection)
	result := col.FindOne(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		return item, ErrNotFound
	} else if err != nil {
		return item, fmt.Errorf("error querying %v: %w", id, err)
	}

	err = result.Decode(&item)
	if err != nil {
		return item, fmt.Errorf("error reading %v: %w", id, err)
	}

	return item, nil
}

func (d radrpDB) UpdateV3ApplicationDefinition(ctx context.Context, application ApplicationResource) (bool, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, application.ID)

	// Creates a new document entry if an existing document with matching ID is not found
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: application.ID}}
	// TODO update this to only update non status values
	update := bson.D{{Key: "$set", Value: application}}

	logger.Info(fmt.Sprintf("Updating Application in DB with operation filter: %s", filter))
	col := d.db.Collection(applicationsV3Collection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating Application: %w", err)
	}

	return (result.UpsertedCount > 0 || result.ModifiedCount > 0), nil
}

func (d radrpDB) DeleteV3Application(ctx context.Context, id azresources.ResourceID) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id.ID)

	// Ensure resources do not exist for this application
	application, err := d.GetV3Application(ctx, id)
	if err != nil {
		return err
	}

	items, err := d.listV3ResourcesByApplication(ctx, id, application, false /* all */)
	if err != nil {
		return err
	}
	if len(items) > 0 {
		return ErrConflict
	}

	// Delete application
	filter := bson.D{{Key: "_id", Value: id.ID}}
	logger.Info(fmt.Sprintf("Deleting Application from DB with operation filter: %s", filter))
	col := d.db.Collection(applicationsV3Collection)
	result := col.FindOneAndDelete(ctx, filter)
	err = result.Err()
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Application with _id: '%s': %w", id, err)
	}

	return nil
}

func (d radrpDB) ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	application, err := d.GetV3Application(ctx, id.Truncate())
	if err != nil {
		return nil, err
	}

	items, err := d.listV3ResourcesByApplication(ctx, id, application, true /* all */)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func (d radrpDB) ListV3Resources(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	application, err := d.GetV3Application(ctx, id.Truncate())
	if err != nil {
		return nil, err
	}

	items, err := d.listV3ResourcesByApplication(ctx, id, application, false /* all */)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func (d radrpDB) listV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID, application ApplicationResource, all bool) ([]RadiusResource, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldResourceID, id.ID)

	items := make([]RadiusResource, 0)

	filter := bson.D{{Key: "subscriptionId", Value: id.SubscriptionID}, {Key: "resourceGroup", Value: id.ResourceGroup},
		{Key: "applicationName", Value: application.ApplicationName}}
	if !all {
		filter = append(filter, bson.E{Key: "type", Value: id.Type()})
	}
	logger.Info(fmt.Sprintf("Listing resources from DB with filter: %s", filter))
	col := d.db.Collection(resourcesCollection)
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return items, fmt.Errorf("error querying resources: %w", err)
	}

	err = cursor.All(ctx, &items)
	if err != nil {
		return items, fmt.Errorf("error reading resources: %w", err)
	}
	logger.Info(fmt.Sprintf("Found %d resources", len(items)))

	return items, nil
}

func (d radrpDB) GetV3Resource(ctx context.Context, id azresources.ResourceID) (RadiusResource, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id,
		radlogger.LogFieldResourceName, id.Name())

	item := RadiusResource{}

	application, err := d.GetV3Application(ctx, id.Truncate())
	if err != nil {
		return item, err
	}

	filter := bson.D{{Key: "subscriptionId", Value: id.SubscriptionID}, {Key: "resourceGroup", Value: id.ResourceGroup},
		{Key: "type", Value: id.Type()}, {Key: "applicationName", Value: application.ApplicationName},
		{Key: "resourceName", Value: id.Name()}}

	logger.Info(fmt.Sprintf("Getting resource from DB with operation filter: %s", filter))
	col := d.db.Collection(resourcesCollection)
	result := col.FindOne(ctx, filter)
	err = result.Err()
	if err == mongo.ErrNoDocuments {
		return item, ErrNotFound
	} else if err != nil {
		return item, fmt.Errorf("error querying %v: %w", id, err)
	}

	err = result.Decode(&item)
	if err != nil {
		return item, fmt.Errorf("error reading %v: %w", id, err)
	}

	return item, nil
}

func (d radrpDB) UpdateV3ResourceDefinition(ctx context.Context, id azresources.ResourceID, resource RadiusResource) (bool, error) {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id,
		radlogger.LogFieldResourceName, id.Name())

	// Creates a new document entry if an existing document with matching ID is not found
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: id.ID}}

	update := bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: resource.ID},
		{Key: "type", Value: resource.Type}, {Key: "subscriptionId", Value: resource.SubscriptionID},
		{Key: "resourceGroup", Value: resource.ResourceGroup}, {Key: "applicationName", Value: resource.ApplicationName},
		{Key: "resourceName", Value: resource.ResourceName}, {Key: "definition", Value: resource.Definition},
		{Key: "provisioningState", Value: resource.ProvisioningState}}}}

	logger.Info(fmt.Sprintf("Updating resource in DB with operation filter: %s", filter))
	col := d.db.Collection(resourcesCollection)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return false, fmt.Errorf("error updating resource: %w", err)
	}

	return (result.UpsertedCount > 0 || result.ModifiedCount > 0), nil
}

func (d radrpDB) UpdateV3ResourceStatus(ctx context.Context, id azresources.ResourceID, resource RadiusResource) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id,
		radlogger.LogFieldResourceName, id.Name())

	// Creates a new document entry if an existing document with matching ID is not found
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: id.ID}}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "status", Value: resource.Status},
			{Key: "computedValues", Value: resource.ComputedValues},
			{Key: "secretValues", Value: resource.SecretValues},
			{Key: "provisioningState", Value: resource.ProvisioningState},
		}},
	}

	logger.Info(fmt.Sprintf("Updating resource status in DB with operation filter: %s", filter))
	col := d.db.Collection(resourcesCollection)
	_, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error updating resource status: %w", err)
	}

	return nil
}

func (d radrpDB) DeleteV3Resource(ctx context.Context, id azresources.ResourceID) error {
	logger := radlogger.GetLogger(ctx).WithValues(radlogger.LogFieldAppID, id,
		radlogger.LogFieldResourceName, id.Name())
	filter := bson.D{{Key: "_id", Value: id.ID}}

	logger.Info(fmt.Sprintf("Deleting resource from DB with operation filter: %s", filter))
	col := d.db.Collection(resourcesCollection)
	result := col.FindOneAndDelete(ctx, filter)
	err := result.Err()
	if err == mongo.ErrNoDocuments {
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting resource with _id: '%s': %w", id, err)
	}

	return nil
}
