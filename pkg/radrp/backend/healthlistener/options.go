// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthlistener

import (
	"context"

	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radrp/hostoptions"
	"go.mongodb.org/mongo-driver/mongo"
)

type ServiceOptions struct {
	DBClientFactory func(ctx context.Context) (*mongo.Database, error)
	HealthChannels  healthcontract.HealthChannels
}

func NewServiceOptions(options hostoptions.HostOptions) ServiceOptions {
	return ServiceOptions{
		DBClientFactory: options.DBClientFactory,
		HealthChannels:  options.HealthChannels,
	}
}
