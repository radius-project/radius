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

package setup

import (
	"time"

	asyncctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/builder"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	mongo_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/mongodatabases"
	rds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/sqldatabases"
	mongo_proc "github.com/radius-project/radius/pkg/datastoresrp/processors/mongodatabases"
	rds_proc "github.com/radius-project/radius/pkg/datastoresrp/processors/rediscaches"
	sql_proc "github.com/radius-project/radius/pkg/datastoresrp/processors/sqldatabases"
	pr_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"
)

const (
	// AsyncOperationRetryAfter is polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// SetupNamespace builds the namespace for core resource provider.
func SetupNamespace(recipeControllerConfig *controllerconfig.RecipeControllerConfig) *builder.Namespace {
	ns := builder.NewNamespace("Applications.Datastores")

	_ = ns.AddResource("redisCaches", &builder.ResourceOption[*datamodel.RedisCache, datamodel.RedisCache]{
		RequestConverter:  converter.RedisCacheDataModelFromVersioned,
		ResponseConverter: converter.RedisCacheDataModelToVersioned,

		Put: builder.Operation[datamodel.RedisCache]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.RedisCache]{
				rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.RedisCache, datamodel.RedisCache](options, &rds_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.RedisCache]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.RedisCache]{
				rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.RedisCache, datamodel.RedisCache](options, &rds_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.RedisCache]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.RedisCache, datamodel.RedisCache](options, &rds_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Custom: map[string]builder.Operation[datamodel.RedisCache]{
			"listsecrets": {
				APIController: rds_ctrl.NewListSecretsRedisCache,
			},
		},
	})

	_ = ns.AddResource("mongoDatabases", &builder.ResourceOption[*datamodel.MongoDatabase, datamodel.MongoDatabase]{
		RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
		ResponseConverter: converter.MongoDatabaseDataModelToVersioned,

		Put: builder.Operation[datamodel.MongoDatabase]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.MongoDatabase]{
				rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.MongoDatabase, datamodel.MongoDatabase](options, &mongo_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.MongoDatabase]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.MongoDatabase]{
				rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.MongoDatabase, datamodel.MongoDatabase](options, &mongo_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.MongoDatabase]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.MongoDatabase, datamodel.MongoDatabase](options, &mongo_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Custom: map[string]builder.Operation[datamodel.MongoDatabase]{
			"listsecrets": {
				APIController: mongo_ctrl.NewListSecretsMongoDatabase,
			},
		},
	})

	_ = ns.AddResource("sqldatabases", &builder.ResourceOption[*datamodel.SqlDatabase, datamodel.SqlDatabase]{
		RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
		ResponseConverter: converter.SqlDatabaseDataModelToVersioned,

		Put: builder.Operation[datamodel.SqlDatabase]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.SqlDatabase]{
				rp_frontend.PrepareRadiusResource[*datamodel.SqlDatabase],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.SqlDatabase, datamodel.SqlDatabase](options, &sql_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Patch: builder.Operation[datamodel.SqlDatabase]{
			UpdateFilters: []apictrl.UpdateFilter[datamodel.SqlDatabase]{
				rp_frontend.PrepareRadiusResource[*datamodel.SqlDatabase],
			},
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewCreateOrUpdateResource[*datamodel.SqlDatabase, datamodel.SqlDatabase](options, &sql_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ResourceClient, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Delete: builder.Operation[datamodel.SqlDatabase]{
			AsyncJobController: func(options asyncctrl.Options) (asyncctrl.Controller, error) {
				return pr_ctrl.NewDeleteResource[*datamodel.SqlDatabase, datamodel.SqlDatabase](options, &sql_proc.Processor{}, recipeControllerConfig.Engine, recipeControllerConfig.ConfigLoader)
			},
			AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
			AsyncOperationRetryAfter: AsyncOperationRetryAfter,
		},
		Custom: map[string]builder.Operation[datamodel.SqlDatabase]{
			"listsecrets": {
				APIController: sql_ctrl.NewListSecretsSqlDatabase,
			},
		},
	})

	// Optional
	ns.SetAvailableOperations(operationList)

	return ns
}
