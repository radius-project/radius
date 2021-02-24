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

	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/curp/revision"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is an error returned when an item is not found in the database.
var ErrNotFound = errors.New("the item was not found")

// ErrConcurrency is an error returned when the item contains stale data and cannot be modified.
var ErrConcurrency = errors.New("the item has been changed")

// CollectionName represents the collection used to store applications in the db.
const collectionName string = "applications"

// NewCurpDB creates a new CurpDB.
func NewCurpDB(m *mongo.Database) CurpDB {
	return curpDB{
		db: m,
	}
}

//go:generate mockgen -destination=../../../mocks/mock_db.go -package=mocks github.com/Azure/radius/pkg/curp/db CurpDB

// CurpDB is our database abstraction.
type CurpDB interface {
	ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]Application, error)
	GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*Application, error)
	PatchApplication(ctx context.Context, patch *ApplicationPatch) error
	DeleteApplicationByID(ctx context.Context, id resources.ApplicationID) error

	ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Component, error)
	GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, rev revision.Revision) (*Component, error)
	PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Component, previous revision.Revision) error
	DeleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error

	ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Deployment, error)
	GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Deployment, error)
	PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Deployment) error
	DeleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error

	ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Scope, error)
	GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Scope, error)
	PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Scope) error
	DeleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error
}

type curpDB struct {
	db *mongo.Database
}

// ListApplicationsByResourceGroup lists applications by (subscription, resource group).
func (d curpDB) ListApplicationsByResourceGroup(ctx context.Context, id resources.ResourceID) ([]Application, error) {
	items := make([]Application, 0)

	filter := bson.D{{Key: "subscriptionId", Value: id.SubscriptionID}, {Key: "resourceGroup", Value: id.ResourceGroup}}
	log.Printf("listing Applications with: %s", filter)
	col := d.db.Collection(collectionName)
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return items, fmt.Errorf("error querying Applications: %w", err)
	}

	err = cursor.All(ctx, &items)
	if err != nil {
		return items, fmt.Errorf("Error reading Applications: %w", err)
	}

	log.Printf("Found %d Applications with: %s", len(items), filter)
	return items, nil
}

// GetApplicationByID finds applications by fully-qualified resource id.
func (d curpDB) GetApplicationByID(ctx context.Context, id resources.ApplicationID) (*Application, error) {
	item := &Application{}

	filter := bson.D{{Key: "_id", Value: id.ID}}
	log.Printf("Getting %v", id)
	col := d.db.Collection(collectionName)
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

func (d curpDB) PatchApplication(ctx context.Context, patch *ApplicationPatch) error {
	options := options.Update().SetUpsert(true)
	filter := bson.D{{Key: "_id", Value: patch.ResourceBase.ID}}
	update := bson.D{{Key: "$set", Value: patch}}

	log.Printf("Updating Application with _id: %s", patch.ResourceBase.ID)
	col := d.db.Collection(collectionName)
	_, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error updating Application: %s", err)
	}

	log.Printf("Updated Application with _id: %s", patch.ResourceBase.ID)
	return nil
}

func (d curpDB) DeleteApplicationByID(ctx context.Context, id resources.ApplicationID) error {
	filter := bson.D{{Key: "_id", Value: id.ID}}

	log.Printf("Deleting Application with _id: %s", id)
	col := d.db.Collection(collectionName)
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

func (d curpDB) ListComponentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Component, error) {
	log.Printf("Listing Components with Application id: %s", id)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]Component, 0, len(application.Components))
	for _, ch := range application.Components {
		if len(ch.RevisionHistory) == 0 {
			log.Printf("Component %s has no revision history.", ch.Name)
			continue
		}

		cr := ch.RevisionHistory[0]
		item := Component{
			ResourceBase: ch.ResourceBase,
			Kind:         cr.Kind,
			Revision:     cr.Revision,
			Properties:   cr.Properties,
		}
		items = append(items, item)
	}

	log.Printf("Found %d Component with Application id: %s", len(application.Components), id)
	return items, nil
}

func (d curpDB) GetComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, rev revision.Revision) (*Component, error) {
	log.Printf("Getting Component with Application id, name, and revision: %s, %s, %s", id, name, rev)
	application, err := d.GetApplicationByID(ctx, id)
	if err != nil {
		return nil, err
	}

	history, ok := application.Components[name]
	if !ok {
		log.Printf("Failed to find Component with Application id, name, and revision: %s, %s, %s", id, name, rev)
		return nil, ErrNotFound
	}

	var cr *ComponentRevision
	if len(history.RevisionHistory) == 0 {
		// no revisions
	} else if rev == revision.Revision("") {
		// "latest", return the first one
		cr = &history.RevisionHistory[len(history.RevisionHistory)-1]
	} else {
		for _, r := range history.RevisionHistory {
			if rev == r.Revision {
				cr = &r
				break
			}
		}
	}

	if cr == nil {
		log.Printf("Failed to find Component with Application id, name, and revision: %s, %s, %s", id, name, rev)
		return nil, ErrNotFound
	}

	item := Component{
		ResourceBase: history.ResourceBase,
		Kind:         cr.Kind,
		Revision:     cr.Revision,
		Properties:   cr.Properties,
	}

	log.Printf("Found Component with Application id, name, and revision: %s, %s, %s", id, name, rev)
	return &item, nil
}

func (d curpDB) PatchComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Component, previous revision.Revision) error {
	col := d.db.Collection(collectionName)

	log.Printf("Updating Component with Application id and name: %s, %s", id, name)

	var filter, update primitive.D

	// If this is the first revision, we need to make sure the component history record exists.
	if previous == revision.Revision("") {
		log.Printf("Updating Component with Application id and name: %s, %s to add component record", id, name)
		ch := &ComponentHistory{
			ResourceBase: patch.ResourceBase,
		}
		key := fmt.Sprintf("components.%s", name)
		filter = bson.D{{Key: "_id", Value: id.ID}, {Key: key, Value: bson.D{{Key: "$exists", Value: false}}}}
		update = bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: ch}}}}
		result, err := col.UpdateOne(ctx, filter, update)
		if err != nil {
			return fmt.Errorf("error updating Component: %s", err)
		}

		log.Printf("Updated Component with Application id and name: %s, %s - %+v to add component record", id, name, result)
	}

	// Now update the component record to add a new history entry
	log.Printf("Updating Component with Application id and name: %s, %s to add component revision", id, name)
	cr := &ComponentRevision{
		Kind:       patch.Kind,
		Revision:   patch.Revision,
		Properties: patch.Properties,
	}

	// Update the document where the revision is the previous revision
	filter = bson.D{{Key: "_id", Value: id.ID}, {Key: fmt.Sprintf("components.%s.revision", name), Value: previous}}
	update = bson.D{
		{Key: "$set", Value: bson.D{{Key: fmt.Sprintf("components.%s.revision", name), Value: cr.Revision}}},
		{Key: "$push", Value: bson.D{{Key: fmt.Sprintf("components.%s.revisionHistory", name), Value: cr}}},
	}
	result, err := col.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("error updating Component: %s", err)
	}

	if result.MatchedCount == 0 {
		log.Printf("Failed to update Component with Application id and name: %s, %s - %+v to add component revision due to concurrency", id, name, result)
		return ErrConcurrency

	}

	log.Printf("Updated Component with Application id and name: %s, %s - %+v to add component revision", id, name, result)
	log.Printf("Updated Component with Application id and name: %s, %s", id, name)

	return nil
}

func (d curpDB) DeleteComponentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("components.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key, Value: ""}}}}

	log.Printf("Deleting Component with Application id and name: %s, %s", id, name)
	col := d.db.Collection(collectionName)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Component with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d curpDB) ListDeploymentsByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Deployment, error) {
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

func (d curpDB) GetDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Deployment, error) {
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

func (d curpDB) PatchDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Deployment) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("deployments.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: patch}}}}

	log.Printf("Updating Deployment with Application id and name: %s, %s", id, name)
	col := d.db.Collection(collectionName)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error updating Application: %s", err)
	}

	log.Printf("Updated Application with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d curpDB) DeleteDeploymentByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("deployments.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key}}}}

	log.Printf("Deleting Deployment with Application id and name: %s, %s, %s", id, name, update)
	col := d.db.Collection(collectionName)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Deployment with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d curpDB) ListScopesByApplicationID(ctx context.Context, id resources.ApplicationID) ([]Scope, error) {
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

func (d curpDB) GetScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) (*Scope, error) {
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

func (d curpDB) PatchScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string, patch *Scope) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("scopes.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: key, Value: patch}}}}

	log.Printf("Updating Scope with Application id and name: %s, %s", id, name)
	col := d.db.Collection(collectionName)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error updating Scope: %s", err)
	}

	log.Printf("Updated Scope with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}

func (d curpDB) DeleteScopeByApplicationID(ctx context.Context, id resources.ApplicationID, name string) error {
	options := options.Update().SetUpsert(true)
	key := fmt.Sprintf("scopes.%s", name)
	filter := bson.D{{Key: "_id", Value: id.ID}}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: key, Value: ""}}}}

	log.Printf("Deleting Scope with Application id and name: %s, %s", id, name)
	col := d.db.Collection(collectionName)
	result, err := col.UpdateOne(ctx, filter, update, options)
	if err != nil {
		return fmt.Errorf("error deleting Application: %s", err)
	}

	log.Printf("Deleted Scope with Application id and name: %s, %s - %+v", id, name, result)
	return nil
}
