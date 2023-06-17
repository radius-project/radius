/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"time"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

var (
	// AsyncCreateOrUpdateRabbitMQTimeout is the timeout for async create or update rabbitMQ
	AsyncCreateOrUpdateRabbitMQTimeout = time.Duration(60) * time.Minute
	// AsyncDeleteRabbitMQTimeout is the timeout for async delete rabbitMQ
	AsyncDeleteRabbitMQTimeout = time.Duration(30) * time.Minute
)

// Options is the options to configure LinkRP controller.
type Options struct {
	ctrl.Options

	// DeployProcessor is the deployment processor for LinkRP
	DeployProcessor deployment.DeploymentProcessor
}
