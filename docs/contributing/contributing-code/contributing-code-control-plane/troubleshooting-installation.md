# Troubleshooting a Radius installation

This page explains some troubleshooting steps that can help you diagnose installation failures or crashes using a custom build of Radius.

## Verify all Radius pods are up and running

```bash
kubectl get pods -n radius-system
```

The installation includes UCP, Applications RP, Dynamic RP, Controller, Deployment Engine, and the PostgreSQL database. The exact pod names contain generated suffixes. Every pod should become `Running` or complete successfully; use `kubectl describe pod <pod-name> -n radius-system` for a pod that remains pending or repeatedly restarts.

## Examine logs

Read logs by deployment name instead of copying an ephemeral pod name:

```bash
kubectl logs deployment/ucp -n radius-system
kubectl logs deployment/applications-rp -n radius-system
kubectl logs deployment/dynamic-rp -n radius-system
kubectl logs deployment/controller -n radius-system
kubectl logs deployment/bicep-de -n radius-system
```

For a specific pod or container, use:

```bash
kubectl logs <pod-name> -n <namespace>
```

Logs with `severity` set to `error` or `warn` could point to why the pods are not in Running state.

Every `rad` CLI error includes a `traceId` that also appears in the control-plane logs. Search all Radius pod logs for that value:

```bash
kubectl logs -n radius-system -l app.kubernetes.io/part-of=radius --all-containers=true --prefix | grep <traceId>
```

See the [observability documentation](https://docs.radapp.io/guides/operations/control-plane/logs/) for more logging guidance.
