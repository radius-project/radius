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

package connections

import (
	"testing"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/stretchr/testify/require"
)

func Test_display(t *testing.T) {
	t.Run("empty graph", func(t *testing.T) {
		graph := []*v20231001preview.ApplicationGraphResource{}
		expected := `Displaying application: cool-app

(empty)

`
		actual := display(graph, "cool-app")
		require.Equal(t, expected, actual)
	})

	t.Run("complex application", func(t *testing.T) {
		sqlRteID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/sql-rte"
		sqlRteType := "Applications.Core/httpRoutes"
		sqlRteName := "sql-rte"

		sqlAppCntrID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/sql-app-ctnr"
		sqlAppCntrName := "sql-app-ctnr"
		sqlAppCntrType := "Applications.Core/containers"

		sqlCntrID := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/sql-ctnr"
		sqlCntrName := "sql-ctnr"
		sqlCntrType := "Applications.Core/containers"

		sqlDbID := "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db"
		sqlDbName := "sql-db"
		sqlDbType := "Applications.Datastores/sqlDatabases"

		provisioningStateSuccess := "Succeeded"
		dirInbound := corerpv20231001preview.DirectionInbound
		dirOutbound := corerpv20231001preview.DirectionOutbound

		graph := []*corerpv20231001preview.ApplicationGraphResource{
			{
				ID:                &sqlRteID,
				Name:              &sqlRteName,
				Type:              &sqlRteType,
				ProvisioningState: &provisioningStateSuccess,
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						ID:        &sqlCntrID,
						Direction: &dirInbound,
					},
				},
			},
			{
				ID:                &sqlCntrID,
				Name:              &sqlCntrName,
				Type:              &sqlCntrType,
				ProvisioningState: &provisioningStateSuccess,
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						Direction: &dirOutbound,
						ID:        &sqlRteID,
					},
				},
			},
			{
				ID:                &sqlDbID,
				Name:              &sqlDbName,
				Type:              &sqlDbType,
				ProvisioningState: &provisioningStateSuccess,
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
			},
			{
				ID:                &sqlAppCntrID,
				Name:              &sqlAppCntrName,
				Type:              &sqlAppCntrType,
				ProvisioningState: &provisioningStateSuccess,
				OutputResources:   []*corerpv20231001preview.ApplicationGraphOutputResource{},
				Connections: []*corerpv20231001preview.ApplicationGraphConnection{
					{
						Direction: &dirInbound,
						ID:        &sqlDbID,
					},
				},
			},
		}

		expected := `Displaying application: test-app

Name: sql-app-ctnr (Applications.Core/containers)
Connections:
  sql-db (Applications.Datastores/sqlDatabases) -> sql-app-ctnr
Resources: (none)

Name: sql-ctnr (Applications.Core/containers)
Connections:
  sql-ctnr -> sql-rte (Applications.Core/httpRoutes)
Resources: (none)

Name: sql-rte (Applications.Core/httpRoutes)
Connections:
  sql-ctnr (Applications.Core/containers) -> sql-rte
Resources: (none)

Name: sql-db (Applications.Datastores/sqlDatabases)
Connections: (none)
Resources: (none)

`
		actual := display(graph, "test-app")
		require.Equal(t, expected, actual)
	})

}
