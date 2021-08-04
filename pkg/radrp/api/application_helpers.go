package api

import "github.com/Azure/radius/pkg/radrp/resources"

func (m *ApplicationResource) GetApplicationID() (resources.ApplicationID, error) {
	id, err := resources.Parse(m.ID)
	if err != nil {
		return resources.ApplicationID{}, err
	}
	return id.Application()
}
