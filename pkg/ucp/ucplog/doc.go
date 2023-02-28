// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

/*
Package ucplog includes the logging helpers to generate log with the radius log schema format.

{
  "severity": "info",
  "timestamp": "2023-02-26T20:14:09.334-0800",
  "name": "applications.core.Applications.Core async worker",
  "scope": "worker/worker.go:279",
  "message": "End processing operation.",
  "resource": {
    "host.name": "appcore-rp",
    "service.name": "applications.core",
    "service.version": "edge"
  },
  "traceId": "d1ba9c7d2326ee1b44eb0b8177ef554f",
  "spanId": "ce52a91ed3c86c6d",
  "attributes": {
    "resourceId": "<ResourceID>",
    "operationId": "0136b023-78c5-440a-b7d3-858120a328f1",
    "operationType": "APPLICATIONS.CORE/CONTAINERS|PUT",
    "dequeueCount": 1,
    "startAt": "2023-02-27T04:14:09.253Z",
    "endAt": "2023-02-27T04:14:09.334Z",
    "duration": 0.08101197
  }
}

# Basic

Radius uses go-logr backed by uber-go/zap logsink to implement strcutured log internally. go-logr offers
well-defined API set and helpers to emit the log without knowing the specific logsink. However,
this is not enough to generate the streamlined logs across the codebase. Package ucplog includes the following
utilities to streamline the log format as well as provide standard developer experiences to the contributers.

- ucplog.FromContextOrDiscard(ctx context.Context)
- ucplog.WithAttributes(ctx context.Context, keysAndValues ...any)
- ucplog.Attributes(ctx context.Context, keysAndValues ...any)

# Example

Radius services such as ucp, corerp, and linkrp injects logger into context during startup and
request-scoped logger for each request. Therefore, we recommend to extract this logger from context
instead of creating new logger.

// Extract logger from context injected previously and add traceId and spanId fields.
// If you do not like to add traceId and spanId to your log, use logr.FromContext(ctx).
logger := ucplog.FromContextOrDiscard(ctx)

// Set the default attributes in the request-scope context.
ctx = ucplog.WithAttributess(ctx,
	"resourceId", "<ResourceID>",
	"operationID", "0136b023-78c5-440a-b7d3-858120a328f1")

...

// Emit log without attributes.
//
// {
//   "severity": "info",
//   "timestamp": "2023-02-26T20:14:09.334-0800",
//   "name": "application.core",
//   "scope": "startup.go:279",
//   "message": "hello radius",
//   "resource": {
//     "host.name": "appcore-rp",
//     "service.name": "applications.core",
//     "service.version": "edge"
//   },
//   "traceId": "d1ba9c7d2326ee1b44eb0b8177ef554f",
//   "spanId": "ce52a91ed3c86c6d",
// }
//
logger.Info("hello radius")

// Emit log with the default attribute in context
//
// {
//   "severity": "info",
//   "timestamp": "2023-02-26T20:14:09.334-0800",
//   "name": "application.core",
//   "scope": "startup.go:279",
//   "message": "hello radius",
//   "resource": {
//     "host.name": "appcore-rp",
//     "service.name": "applications.core",
//     "service.version": "edge"
//   },
//   "traceId": "d1ba9c7d2326ee1b44eb0b8177ef554f",
//   "spanId": "ce52a91ed3c86c6d",
//   "attributes": {
//     "resourceId": "<ResourceID>",
//     "operationId": "0136b023-78c5-440a-b7d3-858120a328f1"
//   }
// }
//
logger.Info("hello radius", ucplog.Attributes(ctx))

// Emit log with the default attribute in context and additional key and values
//
// {
//   "severity": "info",
//   "timestamp": "2023-02-26T20:14:09.334-0800",
//   "name": "application.core",
//   "scope": "startup.go:279",
//   "message": "hello radius",
//   "resource": {
//     "host.name": "appcore-rp",
//     "service.name": "applications.core",
//     "service.version": "edge"
//   },
//   "traceId": "d1ba9c7d2326ee1b44eb0b8177ef554f",
//   "spanId": "ce52a91ed3c86c6d",
//   "attributes": {
//     "resourceId": "<ResourceID>",
//     "operationId": "0136b023-78c5-440a-b7d3-858120a328f1",
//     "additionalKey": "value"
//   }
// }
//
logger.Info("hello radius", ucplog.Attributes(ctx, "additionalKey", "value"))

*/

package ucplog
