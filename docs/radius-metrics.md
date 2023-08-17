# Radius metrics

Radius currently records following custom metrics that provide insight into its health and operations.

  * [Radius async operation metrics](#async-operation-metrics)
  * [Radius recipe metrics](#recipe-metrics)

In addition, Radius uses otelhttp meter provider. Therefore all of the HTTP metrics documented at [otelhttp metrics](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/metrics/semantic_conventions/http-metrics.md) are also available.

## Async operation metrics

| Metrics Name | Description |
|-----|------------|
|asyncoperation.operation | total async operation count
|asyncoperation.queued.operation | number of queued async operation
|asyncoperation.extended.operation | number of extended async operation that are extended
|asyncoperation.duration | async operation duration in milliseconds

## Recipe Engine metrics

| Metrics Name | Description |
|-----|------------|
|recipe.operation.duration | recipe engine operation duration in milliseconds
|recipe.download.duration | recipe download duration in milliseconds
|recipe.tf.installation.duration | Terraform installation duration in milliseconds
|recipe.tf.init.duration | Terraform initialization duration