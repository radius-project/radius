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

package controller

import (
	"time"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

var (
	// AsyncCreateOrUpdateMongoDatabaseTimeout is the timeout for async create or update mongo database
	AsyncCreateOrUpdateMongoDatabaseTimeout = time.Duration(10) * time.Minute
	// AsyncDeleteMongoDatabaseTimeout is the timeout for async delete mongo database
	AsyncDeleteMongoDatabaseTimeout = time.Duration(15) * time.Minute

	// AsyncCreateOrUpdateSqlTimeout is the timeout for async create or update sql database
	AsyncCreateOrUpdateSqlDatabaseTimeout = time.Duration(10) * time.Minute
	// AsyncDeleteSqlDatabaseTimeout is the timeout for async delete sql database
	AsyncDeleteSqlDatabaseTimeout = time.Duration(15) * time.Minute

	// AsyncCreateOrUpdateRedisCacheTimeout is the timeout for async create or update redis cache
	AsyncCreateOrUpdateRedisCacheTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteRedisCacheTimeout is the timeout for async delete redis cache
	AsyncDeleteRedisCacheTimeout = time.Duration(30) * time.Minute

	// AsyncCreateOrUpdateDaprStateStoreTimeout is the timeout for async create or update dapr state store
	AsyncCreateOrUpdateDaprStateStoreTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteDaprStateStoreTimeout is the timeout for async delete dapr state store
	AsyncDeleteDaprStateStoreTimeout = time.Duration(30) * time.Minute
)

// Options is the options to configure LinkRP controller.
type Options struct {
	ctrl.Options

	// DeployProcessor is the deployment processor for LinkRP
	DeployProcessor deployment.DeploymentProcessor
}
