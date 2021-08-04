package api

import "github.com/Azure/radius/pkg/radrp/resources"

func (m *DeploymentResource) GetDeploymentID() (resources.DeploymentID, error) {
	ri, err := resources.Parse(m.ID)
	if err != nil {
		return resources.DeploymentID{}, err
	}

	return ri.Deployment()
}
