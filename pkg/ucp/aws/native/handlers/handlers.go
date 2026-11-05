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

// Package handlers lists the native AWS handlers UCP registers.
package handlers

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	dmssdk "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	daxsdk "github.com/aws/aws-sdk-go-v2/service/dax"
	docdbsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	emrsdk "github.com/aws/aws-sdk-go-v2/service/emr"
	fsxsdk "github.com/aws/aws-sdk-go-v2/service/fsx"

	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/dax"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/dms"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/docdb"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/emr"
	"github.com/radius-project/radius/pkg/ucp/aws/native/handlers/fsx"
)

// All returns the native handlers UCP serves, each using cfg for its AWS clients. Add a handler here when its schema
// is in pkg/aws/schemas.
func All(cfg aws.Config) []native.Handler {
	docdbAPI := docdbsdk.NewFromConfig(cfg)
	return []native.Handler{
		docdb.NewCluster(docdbAPI),
		docdb.NewInstance(docdbAPI),
		dax.NewCluster(daxsdk.NewFromConfig(cfg)),
		dms.NewReplicationInstance(dmssdk.NewFromConfig(cfg)),
		emr.NewCluster(emrsdk.NewFromConfig(cfg)),
		fsx.NewFileSystem(fsxsdk.NewFromConfig(cfg)),
	}
}

// NewRegistry returns the registry of every handler All returns, with the schemas built into Radius.
func NewRegistry(cfg aws.Config) (*native.Registry, error) {
	catalog, err := schemas.Load()
	if err != nil {
		return nil, err
	}
	return native.NewRegistry(catalog, All(cfg)...)
}
