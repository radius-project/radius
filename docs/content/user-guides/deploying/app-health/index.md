---
type: docs
title: "Monitoring application health"
linkTitle: "Monitor app health"
description: "Learn how to use Radius to monitor application health"
weight: 500
---


Radius offers multiple ways to monitor the health of your Application Components, both at the control-plane layer for your hosting provider and at the runtime layer for your services and workloads.

## Container health

The [Container]({{< ref container >}}) running your application code can be configured with [readiness and liveness probes]({{< ref "container.md#container" >}}) to monitor the health of your services. The service code can report its health via HTTP or exec options.

For example, a Container can be configured with a readiness probe:

{{< rad file="snippets/probe.bicep" embed=true marker="//SAMPLE" >}}

The container runtime will probe the path `http://localhost:8080/healthz` to determine if the service is healthy. The service code should implement logic to return a 200 OK response on this HTTP path for reporting readiness of the application.
