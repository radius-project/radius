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
	// TerraformInstallationDuration is the metric name for the Terraform installation duration.
	TerraformInstallationDuration = "terraform.installation.duration"

	// TerraformBinaryDownloadDuration is the metric name for the Terraform binary download duration.
	TerraformBinaryDownloadDuration = "terraform.binary.download.duration"

	// TerraformInitializationDuration is the metric name for the Terraform initialization duration.
	TerraformInitializationDuration = "terraform.initialization.duration"
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

func (m *terraformDriverMetrics) Init() error {
	meter := otel.GetMeterProvider().Meter("terraform-driver-metrics")

	var err error

	m.valueRecorders[TerraformInstallationDuration], err = meter.Float64Histogram(TerraformInstallationDuration)
	if err != nil {
		return err
	}

	m.valueRecorders[TerraformBinaryDownloadDuration], err = meter.Float64Histogram(TerraformBinaryDownloadDuration)
	if err != nil {
		return err
	}

	m.valueRecorders[TerraformInitializationDuration], err = meter.Float64Histogram(TerraformInitializationDuration)
	if err != nil {
		return err
	}

	return nil
}

func (m *terraformDriverMetrics) RecordTerraformInstallationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[TerraformInstallationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[TerraformInstallationDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

func (m *terraformDriverMetrics) RecordTerraformBinaryDownloadDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[TerraformBinaryDownloadDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[TerraformBinaryDownloadDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}

func (m *terraformDriverMetrics) RecordTerraformInitializationDuration(ctx context.Context, startTime time.Time, attrs []attribute.KeyValue) {
	if m.valueRecorders[TerraformInitializationDuration] != nil {
		elapsedTime := float64(time.Since(startTime)) / float64(time.Millisecond)
		m.valueRecorders[TerraformInitializationDuration].Record(ctx, elapsedTime, metric.WithAttributes(attrs...))
	}
}
