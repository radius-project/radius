// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	restapi "github.com/Azure/radius/pkg/radrp/api"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/revision"
)

func newDBResourceBaseFromREST(original rest.ResourceBase) db.ResourceBase {
	return db.ResourceBase{
		ID:             original.ID,
		Name:           original.Name,
		SubscriptionID: original.SubscriptionID,
		ResourceGroup:  original.ResourceGroup,
		Tags:           original.Tags,
		Type:           original.Type,
		Location:       original.Location,
	}
}

func newDBResourceBaseFromTrackedResource(original restapi.TrackedResource) db.ResourceBase {
	return db.ResourceBase{
		ID:       original.ID,
		Name:     original.Name,
		Tags:     original.Tags,
		Type:     original.Type,
		Location: *original.Location,
	}
}

func newRESTResourceBaseFromDB(original db.ResourceBase) rest.ResourceBase {
	return rest.ResourceBase{
		ID:             original.ID,
		Name:           original.Name,
		SubscriptionID: original.SubscriptionID,
		ResourceGroup:  original.ResourceGroup,
		Tags:           original.Tags,
		Type:           original.Type,
		Location:       original.Location,
	}
}

func newResourceFromDB(original db.ResourceBase) restapi.Resource {
	return restapi.Resource{
		ID:   original.ID,
		Name: original.Name,
	}
}

func newTrackedResourceFromDB(original db.ResourceBase) restapi.TrackedResource {
	return restapi.TrackedResource{
		Resource: newResourceFromDB(original),
		Location: &original.Location,
		Tags:     original.Tags,
	}
}

func newRESTApplicationFromDB(original *db.Application) *restapi.ApplicationResource {
	return &restapi.ApplicationResource{
		TrackedResource: newTrackedResourceFromDB(original.ResourceBase),
		Properties:      original.Properties,
	}
}

func newRESTApplicationFromDBPatch(original *db.ApplicationPatch) *restapi.ApplicationResource {
	return &restapi.ApplicationResource{
		TrackedResource: newTrackedResourceFromDB(original.ResourceBase),
		Properties:      original.Properties,
	}
}

func newDBApplicationPatchFromREST(original *restapi.ApplicationResource) *db.ApplicationPatch {
	rb := newDBResourceBaseFromTrackedResource(original.TrackedResource)
	if p, ok := original.Properties.(map[string]interface{}); ok {
		return &db.ApplicationPatch{
			ResourceBase: rb,
			Properties:   p,
		}
	}
	return &db.ApplicationPatch{
		ResourceBase: rb,
	}
}

func toBindingExpression(x *restapi.ComponentBindingExpression) components.BindingExpression {
	return components.BindingExpression{
		Kind:  components.BindingExpressionKind(x.Kind),
		Value: x.Value,
	}
}

func toBindingExpressionMap(x map[string]restapi.ComponentBindingExpression) map[string]components.BindingExpression {
	m := make(map[string]components.BindingExpression)
	for k, v := range x {
		m[k] = toBindingExpression(&v)
	}
	return m
}

func toComponentBindingExpression(x components.BindingExpression) *restapi.ComponentBindingExpression {
	return &restapi.ComponentBindingExpression{
		Kind:  string(x.Kind),
		Value: x.Value,
	}
}

func toComponentBindingExpressionMap(x map[string]components.BindingExpression) map[string]restapi.ComponentBindingExpression {
	m := make(map[string]restapi.ComponentBindingExpression)
	for k, v := range x {
		m[k] = *toComponentBindingExpression(v)
	}
	return m
}

func newDBComponentFromREST(original *restapi.ComponentResource) *db.Component {
	c := &db.Component{
		ResourceBase: newDBResourceBaseFromTrackedResource(original.TrackedResource),
		Kind:         *original.Kind,
		Revision:     revision.Revision(original.Properties.Revision),
		Properties: db.ComponentProperties{
			Build:  original.Properties.Build,
			Config: original.Properties.Config,
			Run:    original.Properties.Run,
			// OutputResources are intentionally not copied over since they are read-only
			OutputResources: []db.OutputResource{},
		},
	}

	for _, d := range original.Properties.Uses {
		dd := db.ComponentDependency{
			Binding: toBindingExpression(d.Binding),
			Env:     toBindingExpressionMap(d.Env),
		}

		if d.Secrets != nil {
			dd.Secrets = &db.ComponentDependencySecrets{
				Store: toBindingExpression(d.Secrets.Store),
				Keys:  toBindingExpressionMap(d.Secrets.Keys),
			}
		}

		c.Properties.Uses = append(c.Properties.Uses, dd)
	}

	c.Properties.Bindings = map[string]db.ComponentBinding{}
	if original.Properties.Bindings != nil {
		for name, b := range original.Properties.Bindings {
			bb := db.ComponentBinding{
				Kind:                 b.Kind,
				AdditionalProperties: b.ComponentBindingAdditionalProperties,
			}
			c.Properties.Bindings[name] = bb
		}
	}

	for _, t := range original.Properties.Traits {
		tt := db.ComponentTrait{
			Kind:                 t.Kind,
			AdditionalProperties: t.ComponentTraitAdditionalProperties,
		}
		c.Properties.Traits = append(c.Properties.Traits, tt)
	}

	return c
}

func newRESTComponentFromDB(original *db.Component) *restapi.ComponentResource {
	c := &restapi.ComponentResource{
		TrackedResource: newTrackedResourceFromDB(original.ResourceBase),
		Kind:            &original.Kind,
		Properties: &restapi.ComponentProperties{
			Revision: string(original.Revision),
			Build:    original.Properties.Build,
			Config:   original.Properties.Config,
			Run:      original.Properties.Run,
		},
	}

	for _, d := range original.Properties.Uses {
		dd := restapi.ComponentDependency{
			Binding: toComponentBindingExpression(d.Binding),
			Env:     toComponentBindingExpressionMap(d.Env),
		}

		if d.Secrets != nil {
			dd.Secrets = &restapi.ComponentDependencySecrets{
				Store: toComponentBindingExpression(d.Secrets.Store),
				Keys:  toComponentBindingExpressionMap(d.Secrets.Keys),
			}
		}
		c.Properties.Uses = append(c.Properties.Uses, &dd)
	}

	c.Properties.Bindings = map[string]restapi.ComponentBinding{}
	if original.Properties.Bindings != nil {
		for name, b := range original.Properties.Bindings {
			bb := restapi.ComponentBinding{
				Kind:                                 b.Kind,
				ComponentBindingAdditionalProperties: b.AdditionalProperties,
			}
			c.Properties.Bindings[name] = bb
		}
	}

	for _, t := range original.Properties.Traits {
		tt := &restapi.ComponentTrait{
			Kind:                               t.Kind,
			ComponentTraitAdditionalProperties: t.AdditionalProperties,
		}
		c.Properties.Traits = append(c.Properties.Traits, tt)
	}

	c.Properties.OutputResources = newComponentOutputResourcesFromDB(original.Properties.OutputResources)
	return c
}

func newDBScopeFromREST(original *rest.Scope) *db.Scope {
	return &db.Scope{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
		Properties:   original.Properties,
	}
}

func newDBScopeFromScopeResource(original *restapi.ScopeResource) *db.Scope {
	return &db.Scope{
		ResourceBase: newDBResourceBaseFromTrackedResource(original.TrackedResource),
		Properties:   original.Properties,
	}
}

func newRESTScopeFromDB(original *db.Scope) *rest.Scope {
	return &rest.Scope{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties:   original.Properties,
	}
}

func newDBDeploymentFromDeploymentResource(original *restapi.DeploymentResource) *db.Deployment {
	d := &db.Deployment{
		ResourceBase: newDBResourceBaseFromTrackedResource(original.TrackedResource),
		Properties: db.DeploymentProperties{
			ProvisioningState: string(original.Properties.ProvisioningState),
		},
	}

	for _, c := range original.Properties.Components {
		cc := &db.DeploymentComponent{
			ID:            c.ID,
			ComponentName: c.ComponentName,
			// We don't allow a REST deployment to specify the revision - it's readonly.
		}

		d.Properties.Components = append(d.Properties.Components, cc)
	}

	return d
}

func newRESTDeploymentFromDB(original *db.Deployment) *rest.Deployment {
	// NOTE: Deployment has some additional state that we don't include in REST responses
	//
	// We track things here like the resources associated with the application as well as
	// any errors that occur during deployment.
	d := &rest.Deployment{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties: rest.DeploymentProperties{
			ProvisioningState: rest.OperationStatus(original.Properties.ProvisioningState),
		},
	}

	for _, c := range original.Properties.Components {
		cc := rest.DeploymentComponent{
			ID:            c.ID,
			ComponentName: c.ComponentName,
			Revision:      c.Revision,
		}

		d.Properties.Components = append(d.Properties.Components, cc)
	}

	return d
}

func newRESTOutputResourcesFromDB(original []db.OutputResource) []rest.OutputResource {
	rrs := []rest.OutputResource{}
	for _, r := range original {
		rr := rest.OutputResource{
			LocalID:            r.LocalID,
			ResourceKind:       r.ResourceKind,
			OutputResourceInfo: r.OutputResourceInfo,
			OutputResourceType: r.OutputResourceType,
			Managed:            r.Managed,
			// Resource includes the body of the resource which would make the REST
			// response too verbose. Hence excluded
		}
		rrs = append(rrs, rr)
	}
	return rrs
}

func newComponentOutputResourcesFromDB(original []db.OutputResource) []*restapi.ComponentOutputResource {
	rrs := []*restapi.ComponentOutputResource{}
	for _, r := range original {
		rr := &restapi.ComponentOutputResource{
			LocalID:            r.LocalID,
			ResourceKind:       r.ResourceKind,
			OutputResourceInfo: r.OutputResourceInfo,
			OutputResourceType: r.OutputResourceType,
			Managed:            r.Managed,
			// Resource includes the body of the resource which would make the REST
			// response too verbose. Hence excluded
		}
		rrs = append(rrs, rr)
	}
	return rrs
}
