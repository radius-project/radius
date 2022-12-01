// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	corerp_dm "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

type radiusScope interface {
	corerp_dm.Environment | corerp_dm.Application
}

// FetchScopeResource fetches environment or application resource linked to resource.
func FetchScopeResource[V radiusScope](ctx context.Context, sp dataprovider.DataStorageProvider, scopeID string, resource resources.ID) (*V, error) {
	id, err := resources.ParseResource(scopeID)
	if err != nil {
		return nil, err
	}

	env := new(V)
	sc, err := sp.GetStorageClient(ctx, id.Type())
	if err != nil {
		return nil, err
	}

	const errMsg = "failed to fetch %q for the resource %q. Error: %w"

	res, err := sc.Get(ctx, id.String())
	if errors.Is(&store.ErrNotFound{}, err) {
		return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("linked %q for resource %s does not exist", scopeID, resource))
	}

	if err != nil {
		return nil, fmt.Errorf(errMsg, scopeID, resource, err)
	}

	err = res.As(env)
	if err != nil {
		return nil, fmt.Errorf(errMsg, scopeID, resource, err)
	}

	return env, nil
}
