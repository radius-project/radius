// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"
	"fmt"
	"sync"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/health/db"
	"github.com/project-radius/radius/pkg/health/handlers"
	"github.com/project-radius/radius/pkg/health/model/azure"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	client_go "k8s.io/client-go/kubernetes"
)

type Service struct {
	Options ServiceOptions
}

func NewService(options ServiceOptions) *Service {
	return &Service{
		Options: options,
	}
}

func (s *Service) Name() string {
	return "health-monitor"
}

func (s *Service) Run(ctx context.Context) error {
	k8s, err := client_go.NewForConfig(s.Options.K8sConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to kubernetes: %w", err)
	}

	dbclient, err := s.Options.DBClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	db := db.NewRadHealthDB(dbclient)

	var arm *armauth.ArmConfig
	if s.Options.Arm != nil {
		// Azure credentials have been provided
		arm = s.Options.Arm
	}
	healthmodel := azure.NewAzureHealthModel(arm, k8s, &sync.WaitGroup{})

	monitorOptions := MonitorOptions{
		Logger:                      radlogger.GetLogger(ctx),
		DB:                          db,
		ResourceRegistrationChannel: s.Options.HealthChannels.ResourceRegistrationWithHealthChannel,
		HealthProbeChannel:          s.Options.HealthChannels.HealthToRPNotificationChannel,
		WatchHealthChangesChannel:   make(chan handlers.HealthState, healthcontract.ChannelBufferSize),
		HealthModel:                 healthmodel,
	}

	healthMonitor := NewMonitor(monitorOptions)
	return healthMonitor.Run(ctx)
}
