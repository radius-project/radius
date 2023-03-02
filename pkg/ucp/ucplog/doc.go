// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

/*
Package ucplog includes the logging helpers to generate log with the radius log schema format.

{
  "timestamp": "2023-02-26T20:14:09.334-0800",
  "severity": "info",
  "name": "applications.core.Applications.Core async worker",
  "caller": "worker/worker.go:279",
  "message": "Hello, Radius.",
  "hostName": "appcore-rp",
  "serviceName": "applications.core",
  "version": "edge"
  "traceId": "d1ba9c7d2326ee1b44eb0b8177ef554f",
  "spanId": "ce52a91ed3c86c6d",
}

# Basic

Radius uses go-logr backed by uber-go/zap logsink to implement strcutured log internally. go-logr offers
well-defined API set and helpers to emit the log without knowing the specific logsink. To enable the
correlation of each logs from request, Radius uses opentelemetry sdk to generate trace id and span id.
To inject trace id and span id into log without additional code, we introduce the below helper:

* FromContextOrDiscard(ctx)

# Examples

logger := ucplog.FromContextOrDiscard(ctx)

...

logger.Info("Hello, Radius.")

*/

package ucplog
