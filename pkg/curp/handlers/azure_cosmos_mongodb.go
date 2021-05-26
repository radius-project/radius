// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/gofrs/uuid"
)

func NewAzureCosmosMongoDBHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosMongoDBHandler{arm: arm}
}

type azureCosmosMongoDBHandler struct {
	arm armauth.ArmConfig
}

func (cddh *azureCosmosMongoDBHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	dac := documentdb.NewDatabaseAccountsClient(cddh.arm.SubscriptionID)
	dac.Authorizer = cddh.arm.Auth

	name, ok := properties["cosmosaccountname"]
	if !ok {
		// names are kinda finicky here - they have to be unique across azure.
		base := properties["name"] + "-"
		name = ""

		for i := 0; i < 10; i++ {
			// 3-24 characters - all alphanumeric and '-'
			uid, err := uuid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to generate storage account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			result, err := dac.CheckNameExists(ctx, name)
			if err != nil {
				return nil, fmt.Errorf("failed to query cosmos account name: %w", err)
			}

			if result.StatusCode == 404 {
				properties["cosmosaccountname"] = name
				break
			}

			log.Printf("cosmos account name generation failed")
		}
	}

	// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
	// in something to do with data locality.
	rgc := resources.NewGroupsClient(cddh.arm.SubscriptionID)
	rgc.Authorizer = cddh.arm.Auth

	g, err := rgc.Get(ctx, cddh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	accountFuture, err := dac.CreateOrUpdate(ctx, cddh.arm.ResourceGroup, name, documentdb.DatabaseAccountCreateUpdateParameters{
		Kind:     documentdb.MongoDB,
		Location: g.Location,
		DatabaseAccountCreateUpdateProperties: &documentdb.DatabaseAccountCreateUpdateProperties{
			DatabaseAccountOfferType: to.StringPtr("Standard"),
			Locations: &[]documentdb.Location{
				{
					LocationName: g.Location,
				},
			},
		},
		Tags: map[string]*string{
			radresources.TagRadiusApplication: &options.Application,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, dac.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	account, err := accountFuture.Result(dac)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb account: %w", err)
	}

	// store account so we can delete later
	properties["cosmosaccountid"] = *account.ID

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	dbfuture, err := mrc.CreateUpdateMongoDBDatabase(ctx, cddh.arm.ResourceGroup, *account.Name, properties["name"], documentdb.MongoDBDatabaseCreateUpdateParameters{
		MongoDBDatabaseCreateUpdateProperties: &documentdb.MongoDBDatabaseCreateUpdateProperties{
			Resource: &documentdb.MongoDBDatabaseResource{
				ID: to.StringPtr(properties["name"]),
			},
			Options: &documentdb.CreateUpdateOptions{
				AutoscaleSettings: &documentdb.AutoscaleSettings{
					MaxThroughput: to.Int32Ptr(4000),
				},
			},
		},
		Tags: map[string]*string{
			radresources.TagRadiusApplication: &options.Application,
			radresources.TagRadiusComponent:   &options.Component,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	db, err := dbfuture.Result(mrc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	// store db so we can delete later
	properties["databasename"] = *db.Name

	return properties, nil
}

func (cddh *azureCosmosMongoDBHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	accountname := properties["cosmosaccountname"]
	dbname := properties["databasename"]

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	// It's possible that this is a retry and we already deleted the account on a previous attempt.
	// When that happens a delete for the database (a nested resource) can fail with a 404, but it's
	// benign.
	dbfuture, err := mrc.DeleteMongoDBDatabase(ctx, cddh.arm.ResourceGroup, accountname, dbname)
	if err != nil && dbfuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	} else if dbfuture.Response().StatusCode != 404 {
		err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
		if err != nil {
			return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
		}

		response, err := dbfuture.Result(mrc)
		if err != nil && response.StatusCode != 404 { // See comment on DeleteMongoDBDatabase
			return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
		}
	}

	dac := documentdb.NewDatabaseAccountsClient(cddh.arm.SubscriptionID)
	dac.Authorizer = cddh.arm.Auth

	accountFuture, err := dac.Delete(ctx, cddh.arm.ResourceGroup, accountname)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, dac.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	_, err = accountFuture.Result(dac)
	if err != nil {
		return fmt.Errorf("failed to DELETE cosmosdb account: %w", err)
	}

	return nil
}
