package api

import "github.com/Azure/radius/pkg/radrp/resources"

func (m *ComponentResource) GetComponentID() (resources.ComponentID, error) {
	ri, err := resources.Parse(m.ID)
	if err != nil {
		return resources.ComponentID{}, err
	}

	return ri.Component()
}
