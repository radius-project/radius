---
type: docs
title: "Monitoring application health"
linkTitle: "Monitoring application health"
description: "Learn how to use Radius to monitor application health"
weight: 500
---

The container running the application code could be configured with readiness and liveness probes that can be used by consumers to monitor the health of the application. The application code can report its health on the readiness/liveness endpoint configured.

For example, for the readiness probe config as follows:-

```json
readinessProbe:{
    kind:'httpGet'
    containerPort:8080
    path: '/readyz'
    initialDelaySeconds:3
    failureThreshold:4
    periodSeconds:20
}
```

The container runtime will probe the path http://localhost:8080/readyz to determine if the application is healthy. The application code should implement logic to return a 200 OK response on this HTTP path for reporting readiness of the application.