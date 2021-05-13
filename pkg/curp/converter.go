// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package curp

import (
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/rest"
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
		Properties:   original.Properties,
	}
}

func newRESTApplicationFromDBPatch(original *db.ApplicationPatch) *rest.Application {
	return &rest.Application{
		ResourceBase: newRESTResourceBaseFromDB(original.ResourceBase),
		Properties:   original.Properties,
	}
}

func newDBApplicationPatchFromREST(original *rest.Application) *db.ApplicationPatch {
	return &db.ApplicationPatch{
		ResourceBase: newDBResourceBaseFromREST(original.ResourceBase),
		Properties:   original.Properties,
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
		},
	}

	for _, d := range original.Properties.DependsOn {
		dd := db.ComponentDependsOn{
			Name:      d.Name,
			Kind:      d.Kind,
			SetEnv:    d.SetEnv,
			SetSecret: d.SetSecret,
		}
		c.Properties.DependsOn = append(c.Properties.DependsOn, dd)
	}

	for _, p := range original.Properties.Provides {
		pp := db.ComponentProvides{
			Name:          p.Name,
			Kind:          p.Kind,
			Port:          p.Port,
			ContainerPort: p.ContainerPort,
		}
		c.Properties.Provides = append(c.Properties.Provides, pp)
	}

	for _, t := range original.Properties.Traits {
		tt := db.ComponentTrait{
			Kind:       t.Kind,
			Properties: t.Properties,
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

	for _, d := range original.Properties.DependsOn {
		dd := rest.ComponentDependsOn{
			Name:   d.Name,
			Kind:   d.Kind,
			SetEnv: d.SetEnv,
		}
		c.Properties.DependsOn = append(c.Properties.DependsOn, dd)
	}

	for _, p := range original.Properties.Provides {
		pp := rest.ComponentProvides{
			Name:          p.Name,
			Kind:          p.Kind,
			ContainerPort: p.ContainerPort,
		}
		c.Properties.Provides = append(c.Properties.Provides, pp)
	}

	for _, t := range original.Properties.Traits {
		tt := rest.ComponentTrait{
			Kind:       t.Kind,
			Properties: t.Properties,
		}
		c.Properties.Traits = append(c.Properties.Traits, tt)
	}

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
			Revision:      c.Revision,
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
