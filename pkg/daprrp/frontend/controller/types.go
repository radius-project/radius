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
)

var (
	// AsyncCreateOrUpdateDaprStateStoreTimeout is the timeout for async create or update dapr state store
	AsyncCreateOrUpdateDaprStateStoreTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteDaprStateStoreTimeout is the timeout for async delete dapr state store
	AsyncDeleteDaprStateStoreTimeout = time.Duration(30) * time.Minute

	// AsyncCreateOrUpdateDaprSecretStoreTimeout is the timeout for async create or update dapr secret store
	AsyncCreateOrUpdateDaprSecretStoreTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteDaprSecretStoreTimeout is the timeout for async delete dapr secret store
	AsyncDeleteDaprSecretStoreTimeout = time.Duration(30) * time.Minute

	// AsyncCreateOrUpdateDaprPubSubBrokerTimeout is the timeout for async create or update dapr pub sub broker
	AsyncCreateOrUpdateDaprPubSubBrokerTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteDaprPubSubBrokerTimeout is the timeout for async delete dapr pub sub broker
	AsyncDeleteDaprPubSubBrokerTimeout = time.Duration(30) * time.Minute
)
