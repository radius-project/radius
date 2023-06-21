## Troubleshooting

This page explains some troubleshooting steps that can help you diagnose installation failures or crashes using a custom build of Radius.

1. Verify all Radius pods are up and running
```
kubectl get pods -n radius-system 

NAME                               READY   STATUS    RESTARTS   AGE
appcore-rp-765d7c697f-sd7qk        1/1     Running   0          2d1h
bicep-de-5c4b5565bc-hm6dm          1/1     Running   0          2d1h
contour-contour-7c88c58fc8-wgpcv   1/1     Running   0          2d1h
contour-envoy-xnqxh                2/2     Running   0          2d1h
ucp-fc695d88-drctw                 1/1     Running   0          2d1h
```
All of the pods in the radius-system namespace should be in the Running state if your installation is working correct. If you see pods that are not in the running state, then this indicates a failure.

2. Look at logs 

You could look at logs on each pod to understand what went wrong by using
```
kubectl logs <pod-name> -n <namespace>
```

Use below commands to see ucp, appcore-rp and bicep-de logs respectively
```
kubectl logs ucp-fc695d88-drctw -n radius-system #replace ucp-fc695d88-drctw with name of your pod
kubectl logs appcore-rp-765d7c697f-sd7qk -n radius-system #replace appcore-rp-765d7c697f-sd7qk with name of your pod
kubectl logs bicep-de-5c4b5565bc-hm6dm -n radius-system #replace bicep-de-5c4b5565bc-hm6dm with name of your pod
```

Logs with  `severity` set to `error` or `warn` could point to why the pods are not in Running state.

Every rad CLI command  outputs a `traceId` in the event of error. These identifiers correspond to the `traceId` field that appears in the log messages produced by Radius.
By searching the logs for the `traceId` value, you can find the logs associated with a request that failed
You can learn more about observability and logging in Radius in our [observability docs](https://docs.radapp.dev/operations/control-plane/observability/logging/logs/).
```
{
  "severity": "info",
  "timestamp": "2023-06-14T19:37:05.849Z",
  "name": "applications.core.Applications.Core",
  "caller": "applications/updatefilter.go:129",
  "message": "Created the namespace",
  "serviceName": "applications.core",
  "version": "edge",
  "hostName": "appcore-rp-765d7c697f-sd7qk",
  "resourceId": "/planes/radius/local/resourcegroups/default/providers/applications.core/applications/deploymentengine",
  "traceId": "a459ab8af8166a17cfc181d4d01fa19a",
  "spanId": "5f68d8ffa02b8924",
  "namespace": "default-deploymentengine"
}
```

