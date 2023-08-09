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

package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// TerraformInstallationDurationMilliseconds is the metric name for the Terraform installation duration.
	TerraformInstallationDurationMilliseconds = "terraform_installation_duration_milliseconds"

	// TerraformInitializationDurationMilliseconds is the metric name for the Terraform initialization duration.
	TerraformInitializationDurationMilliseconds = "terraform_initialization_duration_milliseconds"

	// TerraformVersionAttrKey is the attribute key for the Terraform version.
	TerraformVersionAttrKey = "terraform.version"
)

type terraformDriverMetrics struct {
	counters       map[string]metric.Int64Counter
	valueRecorders map[string]metric.Float64Histogram
}

func newTerraformDriverMetrics() *terraformDriverMetrics {
	return &terraformDriverMetrics{
		counters:       make(map[string]metric.Int64Counter),
		valueRecorders: make(map[string]metric.Float64Histogram),
	}
}

// Init initializes the Terraform Driver metrics.
func (m *terraformDriverMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("terraform-driver-metrics")

	var err error

	m.valueRecorders[TerraformInstallationDurationMilliseconds], err = meter.Float64Histogram(TerraformInstallationDurationMilliseconds)
	if err != nil {
		return err
	}

	m.valueRecorders[TerraformInitializationDurationMilliseconds], err = meter.Float64Histogram(TerraformInitializationDurationMilliseconds)
	if err != nil {
		return err
	}

	return nil
}

// RecordTerraformInstallationDuration records the duration of the Terraform installation with the given attributes.
func (m *terraformDriverMetrics) RecordTerraformInstallationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[TerraformInstallationDurationMilliseconds] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[TerraformInstallationDurationMilliseconds].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

// RecordTerraformInitializationDuration records the Terraform initialization duration with the given attributes.
func (m *terraformDriverMetrics) RecordTerraformInitializationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[TerraformInitializationDurationMilliseconds] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[TerraformInitializationDurationMilliseconds].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}
