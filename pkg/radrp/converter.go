// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	radhealthdb "github.com/Azure/radius/pkg/health/db"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/rest"
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

func newRESTApplicationFromDB(original *db.Application) *rest.Application {
	return &rest.Application{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties: rest.ApplicationProperties{
			Status: newRESTApplicationStatusFromDB(original),
		},
	}
}

func newRESTApplicationStatusFromDB(original *db.Application) rest.ApplicationStatus {
	return rest.ApplicationStatus{
		ProvisioningState:        original.Properties.Status.ProvisioningState,
		ProvisioningErrorDetails: original.Properties.Status.ProvisioningErrorDetails,
		HealthState:              original.Properties.Status.HealthState,
		HealthErrorDetails:       original.Properties.Status.HealthErrorDetails,
	}
}

func newRESTApplicationFromDBPatch(original *db.ApplicationPatch) *rest.Application {
	return &rest.Application{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties: rest.ApplicationProperties{
			Status: rest.ApplicationStatus{
				ProvisioningState:        original.Properties.Status.ProvisioningState,
				ProvisioningErrorDetails: original.Properties.Status.ProvisioningErrorDetails,
				HealthState:              original.Properties.Status.HealthState,
				HealthErrorDetails:       original.Properties.Status.HealthErrorDetails,
			},
		},
	}
}

func newDBApplicationPatchFromREST(original *rest.Application) *db.ApplicationPatch {
	return &db.ApplicationPatch{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
		Properties: db.ApplicationProperties{
			Status: db.ApplicationStatus{
				ProvisioningState:        original.Properties.Status.ProvisioningState,
				ProvisioningErrorDetails: original.Properties.Status.ProvisioningErrorDetails,
				HealthState:              original.Properties.Status.HealthState,
				HealthErrorDetails:       original.Properties.Status.HealthErrorDetails,
			},
		},
	}
}

func newDBComponentFromREST(original *rest.Component) *db.Component {
	c := &db.Component{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
		Kind:         original.Kind,
		Revision:     original.Properties.Revision,
		Properties: db.ComponentProperties{
			Build:  original.Properties.Build,
			Config: original.Properties.Config,
			Run:    original.Properties.Run,
			// Status is intentionally not copied over since it is read-only
			Status: db.ComponentStatus{
				ProvisioningState: db.NotProvisioned,
				HealthState:       radhealthdb.Unhealthy,
				OutputResources:   []db.OutputResource{},
			},
		},
	}

	for _, d := range original.Properties.Uses {
		dd := db.ComponentDependency{
			Binding: d.Binding,
			Env:     d.Env,
		}

		if d.Secrets != nil {
			dd.Secrets = &db.ComponentDependencySecrets{
				Store: d.Secrets.Store,
				Keys:  d.Secrets.Keys,
			}
		}

		c.Properties.Uses = append(c.Properties.Uses, dd)
	}

	c.Properties.Bindings = map[string]db.ComponentBinding{}
	if original.Properties.Bindings != nil {
		for name, b := range original.Properties.Bindings {
			bb := db.ComponentBinding{
				Kind:                 b.Kind,
				AdditionalProperties: b.AdditionalProperties,
			}
			c.Properties.Bindings[name] = bb
		}
	}

	for _, t := range original.Properties.Traits {
		tt := db.ComponentTrait{
			Kind:                 t.Kind,
			AdditionalProperties: t.AdditionalProperties,
		}
		c.Properties.Traits = append(c.Properties.Traits, tt)
	}

	return c
}

func newRESTComponentFromDB(original *db.Component) *rest.Component {
	c := &rest.Component{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Kind:         original.Kind,
		Properties: rest.ComponentProperties{
			Revision: original.Revision,
			Build:    original.Properties.Build,
			Config:   original.Properties.Config,
			Run:      original.Properties.Run,
		},
	}

	for _, d := range original.Properties.Uses {
		dd := rest.ComponentDependency{
			Binding: d.Binding,
			Env:     d.Env,
		}

		if d.Secrets != nil {
			dd.Secrets = &rest.ComponentDependencySecrets{
				Store: d.Secrets.Store,
				Keys:  d.Secrets.Keys,
			}
		}
		c.Properties.Uses = append(c.Properties.Uses, dd)
	}

	c.Properties.Bindings = map[string]rest.ComponentBinding{}
	if original.Properties.Bindings != nil {
		for name, b := range original.Properties.Bindings {
			bb := rest.ComponentBinding{
				Kind:                 b.Kind,
				AdditionalProperties: b.AdditionalProperties,
			}
			c.Properties.Bindings[name] = bb
		}
	}

	for _, t := range original.Properties.Traits {
		tt := rest.ComponentTrait{
			Kind:                 t.Kind,
			AdditionalProperties: t.AdditionalProperties,
		}
		c.Properties.Traits = append(c.Properties.Traits, tt)
	}

	c.Properties.Status = newRESTComponentStatusFromDB(original)
	return c
}

func newDBScopeFromREST(original *rest.Scope) *db.Scope {
	return &db.Scope{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
		Properties:   original.Properties,
	}
}

func newRESTScopeFromDB(original *db.Scope) *rest.Scope {
	return &rest.Scope{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties:   original.Properties,
	}
}

func newDBDeploymentFromREST(original *rest.Deployment) *db.Deployment {
	d := &db.Deployment{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
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
			HealthID:           r.HealthID,
			Status: rest.OutputResourceStatus{
				HealthState:        r.Status.HealthState,
				HealthErrorDetails: r.Status.HealthStateErrorDetails,
			},
			// Resource includes the body of the resource which would make the REST
			// response too verbose. Hence excluded
		}
		rrs = append(rrs, rr)
	}
	return rrs
}

func newRESTComponentStatusFromDB(original *db.Component) rest.ComponentStatus {
	status := rest.ComponentStatus{
		ProvisioningState: original.Properties.Status.ProvisioningState,
		HealthState:       original.Properties.Status.HealthState,
		OutputResources:   newRESTOutputResourcesFromDB(original.Properties.Status.OutputResources),
	}
	return status
}
