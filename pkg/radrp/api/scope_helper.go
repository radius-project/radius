package api

import "github.com/Azure/radius/pkg/radrp/resources"

func (m *ScopeResource) GetScopeID() (resources.ScopeID, error) {
	ri, err := resources.Parse(m.ID)
	if err != nil {
		return resources.ScopeID{}, err
	}

	return ri.Scope()
}
