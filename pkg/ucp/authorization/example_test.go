// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

func ExampleEvaluator() {
	catalog, err := NewCatalog(nil)
	if err != nil {
		panic(err)
	}
	principal := api.Principal{
		Type: to.Ptr(api.PrincipalTypeUser), Issuer: to.Ptr("kubernetes:cluster-a"), Subject: to.Ptr("alice"),
	}
	group := "/planes/radius/local/resourceGroups/team-a"
	assignments := []api.RoleAssignmentResource{{
		ID: to.Ptr(group + "/providers/Radius.Core/roleAssignments/developer"),
		Properties: &api.RoleAssignmentProperties{
			Principal:        &principal,
			RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/application-developer"),
		},
	}}
	evaluator, err := NewEvaluator(catalog, nil, assignments)
	if err != nil {
		panic(err)
	}
	decisions, err := evaluator.Evaluate(Identity{Principal: principal}, []Check{
		{"Radius.Core/applications/write", group + "/providers/Radius.Core/applications/app"},
		{"Radius.Core/environments/use/action", group + "/providers/Radius.Core/environments/staging"},
		{"System.Azure/credentials/use/action", "/planes/azure/local"},
		{"Radius.Core/applications/write", "/planes/radius/local/resourceGroups/team-b/providers/Radius.Core/applications/app"},
	})
	if err != nil {
		panic(err)
	}
	for _, decision := range decisions {
		fmt.Printf("%s: %t\n", decision.Permission, decision.Allowed)
	}
	// Output:
	// Radius.Core/applications/write: true
	// Radius.Core/environments/use/action: false
	// System.Azure/credentials/use/action: false
	// Radius.Core/applications/write: false
}
