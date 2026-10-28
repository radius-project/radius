# Introduction

The Radius helm chart deploys the Radius services on a Kubernetes cluster using Helm.

## Prerequisites

- Kubernetes cluster with RBAC enabled
- Helm 3

## Installing the Chart

To install the chart with the release name `radius`:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system
```

## Configuration Options

### Flux source selection

Flux reconciliation is disabled by default. Enable it by selecting each `GitRepository` with its exact namespace, name, and Kubernetes UID:

```yaml
controller:
  flux:
    enabled: true
    sources:
      - namespace: flux-system
        name: applications
        uid: "<metadata.uid of the GitRepository>"
```

Read the UID from the existing object, not from a manifest or a previous object with the same name:

```console
kubectl get gitrepositories.source.toolkit.fluxcd.io applications -n flux-system -o jsonpath='{.metadata.uid}'
```

An empty list selects no sources. Duplicate namespace/name entries, missing fields, and wildcard identities are rejected, including in disabled configuration. Invalid settings stop controller startup, so render reviewed values with `helm template` before applying them. Use the matching controller image and chart; older controller binaries do not understand these options.

#### Before enabling

Selecting a source trusts its accepted publishers with the installation's deployment capabilities. The default deployment engine uses `cluster-admin` and supports arbitrary Kubernetes resources; selection does not limit a template to one namespace or provide tenant isolation. Trust must extend to executable content from included repositories, submodules, Bicep modules, and any other accepted dependencies.

Only installation administrators should modify source specifications, referenced verification keys and included sources, or controller configuration. DeploymentTemplate specifications and status must be writable only by the controller and installation-trusted operators. Verify the actual user and group permissions before enabling sources; see the [configuration guide](../../docs/contributing/contributing-code/contributing-code-control-plane/configSettings.md#flux-source-configuration). A selected namespace/name/UID is not a substitute for those write restrictions.

The source must be active and Ready for its current generation with an artifact. An administrator's source-specification change holds older generated templates until the new generation is ready and reconciled. Source deletion/recreation requires an explicit identity update. A selected source in `flux-system` may deploy templates into other namespaces.

Settings are read at process startup. The chart changes a pod checksum and uses `Recreate` for the single controller replica, avoiding overlap between old and new selections. This briefly pauses all controller reconcilers. A configuration change takes full effect only after the old controller has stopped and the new pod is ready.

#### Target changes and cleanup

Moving an existing entry to another namespace while retaining its template name and Radius deployment scope is held while the old DeploymentTemplate exists, including while it is deleting. First remove the entry and wait for its template and resources to finish deletion, then add it in the new namespace. This sequence deletes and recreates the workload; it is not a live transfer. Initial cross-namespace targets and moves to a different deployment scope remain supported.

New deployments wait for previous tracked cleanup to finish unless an application or environment is waiting on live child resources owned by the same template. In that case, changed desired content can proceed so it can remove those dependencies. If the update restores the parent before its backend deletion has started, the controller replaces only the terminating tracking object and retains the backend resource and any still-desired children.

`ReadyPendingCleanup` means cleanup is still outstanding; it does not mean all deletes have started. Already submitted deletes must finish before conflicting desired outputs are reapplied and their tracking is restored. Do not strip finalizers to bypass cleanup.

Deleting or pruning a template does not discard a completed deployment's output inventory. Missing output trackers are recovered for cleanup before the saved operation is cleared, including when deletion interrupts tracker replacement. In a terminating namespace, existing trackers are reused and missing outputs are instead retained in the template's `status.pendingCleanup`, including resumable deletion operations. The template remains until both inventories finish cleanup; inactive sources still hold these effects.

#### Migrating existing templates

Existing source-managed templates with only `spec.repository` are held until an administrator reviews and associates them with a source. `Held` does not delete running resources; it retains tracking objects and finalizers, which can delay namespace deletion. Direct templates with neither source field continue through the existing trusted Kubernetes-writer workflow.

Use an installation-specific, reviewed chart directory and values file. Keep the inventory and patch files private: templates and parameters can contain application data.

1. Inventory source identities, template UIDs/specifications, tracked resource IDs, and in-progress operation IDs. Pause the controller and wait for its pods to stop:

   ```console
   kubectl scale deployment/controller -n radius-system --replicas=0
   kubectl wait --for=delete pod -n radius-system -l app.kubernetes.io/name=controller --timeout=120s
   ```

2. Apply the DeploymentTemplate CRD from the same reviewed chart version as the new controller. Helm does not upgrade existing definitions from its `crds` directory. Install the new build with Flux disabled:

   ```sh
   CHART=deploy/Chart
   kubectl apply -f "$CHART/crds/radius/radapp.io_deploymenttemplates.yaml"
   helm upgrade radius "$CHART" -n radius-system -f reviewed-values.yaml \
     --set controller.flux.enabled=false --set-json 'controller.flux.sources=[]' --wait
   ```

3. For each reviewed template, read the current source and template as JSON. Confirm source readiness and compare the template, parameters, provider configuration, and resource inventory with the approved source content. Generate a JSON Patch that tests the template UID and resource version before adding attribution; do not delete and recreate the template:

   ```sh
   kubectl get gitrepositories.source.toolkit.fluxcd.io "$SOURCE" -n "$SOURCE_NAMESPACE" -o json > source.json
   kubectl get deploymenttemplates.radapp.io "$TEMPLATE" -n "$TEMPLATE_NAMESPACE" -o json > template.json
   jq -n --slurpfile source source.json --slurpfile template template.json '
     $source[0] as $s | $template[0] as $t |
     [
       {op: "test", path: "/metadata/uid", value: $t.metadata.uid},
       {op: "test", path: "/metadata/resourceVersion", value: $t.metadata.resourceVersion},
       {op: "add", path: "/spec/repository", value: $s.metadata.name},
       {op: "add", path: "/spec/sourceRef", value: {
         namespace: $s.metadata.namespace, name: $s.metadata.name,
         uid: $s.metadata.uid, generation: $s.metadata.generation,
         revision: $s.status.artifact.revision, digest: $s.status.artifact.digest
       }}
     ]' > association.json
   kubectl patch deploymenttemplates.radapp.io "$TEMPLATE" -n "$TEMPLATE_NAMESPACE" \
     --type=json --patch-file association.json
   ```

4. Read both objects again. Confirm the association, unchanged template UID/business specification/finalizers, and unchanged resource IDs. Enable the reviewed source list and wait for the new controller:

   ```sh
   helm upgrade radius "$CHART" -n radius-system -f reviewed-selected-values.yaml --wait
   kubectl rollout status deployment/controller -n radius-system --timeout=180s
   kubectl get deploymenttemplates.radapp.io -A
   ```

Templates should leave `Held` only when their current source and attribution match. Use the same reviewed in-place association for an intentional transfer to a recreated source; never infer ownership from a reused name. Clean up through an active approved source before retiring it. If a transfer is not approved, leave objects held and choose recovery explicitly rather than removing finalizers.

#### Pausing and recovery

Suspend a source and remove it from the selected list to prevent new source-managed effects. Complete the controller rollout, or stop all controller pods for a stronger controller-side pause. Source-managed deletion is also held. Previously accepted deployment-engine operations can continue independently; polling and tracking them is not cancellation.

Prefer the new controller build with Flux disabled as the fallback. Do not use ordinary `helm rollback` or a pre-change controller image to restore service: an older binary either rejects the new configuration or resumes its old source processing with old configuration. Exceptional recovery with an older build must keep the entire controller stopped until a maintainer-approved compatible build is available.

### In-Cluster Container Image Builds

The `Radius.Compute/containerImages` resource type requires the opt-in rootless BuildKit sidecar:

```console
rad install kubernetes \
  --set dynamicrp.buildkit.enabled=true
```

Git build sources require no additional storage. To enable local filesystem build sources, create a PVC in the Radius release namespace containing the approved source directories, then configure the chart to mount it read-only:

```console
rad install kubernetes \
  --set dynamicrp.buildkit.enabled=true \
  --set dynamicrp.buildkit.localContexts.existingClaim=build-contexts
```

The chart does not create or populate the PVC. It mounts the claim only into `dynamic-rp` at `/var/radius/build-contexts`; local source paths must resolve beneath that directory.

`dynamic-rp` reads the mount as user ID `65532` and the Pod sets no `fsGroup`, so every directory and file on the claim must be readable by that user or world-readable. The Deployment runs a single replica with the default rolling update, so a `ReadWriteOnce` claim can block a rollout when the replacement Pod is scheduled to another node. Prefer a `ReadOnlyMany` or `ReadWriteMany` claim where the storage class supports it.

BuildKit defaults to one parallel OCI worker step across all image builds sharing the `dynamic-rp` Pod. This protects the sidecar's memory limit when multiple `containerImages` resources are deployed together. `dynamicrp.buildkit.maxParallelism` must be an integer between `1` and `2147483647`; BuildKit treats non-positive values as unlimited, so the chart rejects them before rendering the daemon configuration.

Increase `dynamicrp.buildkit.maxParallelism` only after profiling representative cold builds. Size `dynamicrp.buildkit.resources.requests` for the sustained working set and `dynamicrp.buildkit.resources.limits` with enough headroom for the configured parallelism. The limit bounds concurrent build steps but cannot make an individual build fit within an undersized memory limit.

Do not lower the global `workerServer.maxOperationConcurrency` to control image-build memory. That setting governs every Dynamic RP operation and does not constrain parallel steps within a single BuildKit solve.

### Async worker polling

UCP, Applications RP and Dynamic RP poll their queue for async operations. With the default API server queue, each poll is a request to the Kubernetes API server, so an idle installation polls about five times per second per component. Set `ucp.workerServer.dequeueInterval`, `rp.workerServer.dequeueInterval` or `dynamicrp.workerServer.dequeueInterval` to a duration such as `2s` to poll an empty queue less often. An empty value or a non-positive duration keeps the 200ms default.

The interval applies only after a poll finds the queue empty or fails; a worker polls again immediately after it dequeues an operation. A longer interval therefore delays the start of the first operation after an idle period by up to that interval, but does not slow down a burst of operations. The components read the setting at startup, so restart their Deployments after changing it on an existing installation.

### Custom Container Registry

By default, Radius pulls container images from GitHub Container Registry (ghcr.io). For air-gapped environments or when using private registries, you can configure a custom container registry using the `global.imageRegistry` parameter.

### Custom Image Tag

**Default image tags are version-pinned.** Final and patch charts use the full `Chart.AppVersion` (for example, `0.61.0` or `0.61.2`), including the Deployment Engine, dashboard, Bicep, readiness and pre-upgrade images. RC charts continue to use their full RC version. Historical charts keep their original tag behavior.

**Patch updates require a chart upgrade.** Restarting pods no longer picks up a later patch through the moving `major.minor` alias. Upgrade the Radius CLI and run `rad upgrade kubernetes`, or upgrade to the desired chart version with Helm. Channel aliases remain published for existing consumers.

An upgrade adopts the new defaults when the previous installation used chart defaults. Explicit `global.imageTag`, component tags, and tagged or digest-pinned image paths remain respected. To remove a stored global channel override while preserving other user settings, use `rad upgrade kubernetes --set global.imageTag=`. Clear component-specific tag overrides the same way. An empty image value renders an invalid image reference, so restore each overridden image to its default name from `values.yaml` instead, for example `--set rp.image=applications-rp`. `--reset-values` restores every default but also discards unrelated stored settings.

You can specify a custom tag for all Radius images using the `global.imageTag` parameter. This is useful when you want to deploy a specific version across all components or use custom-built images.

Main-branch Radius images use the mutable `edge` tag. The `latest` tag points to the most recent stable release. Use `edge` when you need builds from `main`.

The Deployment Engine and dashboard are published from separate repositories. Edge charts continue to use their existing `latest` tags until those publishers adopt the `edge` convention.

#### Using a Custom Registry

```console
# Using Azure Container Registry
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io

# Using AWS Elastic Container Registry
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=123456789.dkr.ecr.us-west-2.amazonaws.com

# Using a private registry with custom port
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=private.registry.com:5000
```

#### Using a Custom Image Tag

```console
# Use a specific version for all components
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageTag=0.48

# Combine custom registry with custom tag
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# Override specific component while using global tag for others
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageTag=0.48 \
  --set controller.tag=0.49
```

#### With rad CLI commands

The custom registry and tag configuration is also supported in rad CLI commands:

```console
# During initial installation with custom registry
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io

# During initial installation with custom tag
rad install kubernetes \
  --set global.imageTag=0.48

# Combine custom registry and tag
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# During upgrade
rad upgrade kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48

# During initialization
rad init \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set global.imageTag=0.48
```

#### Using with Private Registries and Certificates

For private registries that require custom CA certificates:

```console
# Install with custom CA certificate
rad install kubernetes \
  --set global.imageRegistry=private.registry.com \
  --set-file global.rootCA.cert=/path/to/ca-certificate.pem

# Or using Helm directly
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=private.registry.com \
  --set-file global.rootCA.cert=/path/to/ca-certificate.pem
```

#### Using with Private Registries and Authentication

For private registries that require authentication:

1. Create the docker-registry secret in the `radius-system` namespace:

```bash
kubectl create secret docker-registry regcred \
  --docker-server=myregistry.azurecr.io \
  --docker-username=<username> \
  --docker-password=<password> \
  --docker-email=<email> \
  -n radius-system
```

1. Reference the secret in your Helm values:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set-string 'global.imagePullSecrets[0].name=regcred'

# Or using values file:
# values.yaml:
# global:
#   imageRegistry: myregistry.azurecr.io
#   imagePullSecrets:
#     - name: regcred
```

1. With rad CLI:

```console
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io \
  --set 'global.imagePullSecrets[0].name=regcred'
```

#### Air-gapped Environment Setup

For completely air-gapped environments, you'll need to:

1. Mirror all Radius images to your private registry
2. Configure Radius to use your private registry
3. Create and reference image pull secrets if authentication is required

Example of mirroring images (requires access to both registries):

```bash
# List of Radius images
IMAGES=(
  "controller"
  "ucpd"
  "applications-rp"
  "dynamic-rp"
  "deployment-engine"
  "dashboard"
  "bicep"
)

SOURCE_REGISTRY="ghcr.io/radius-project"
TARGET_REGISTRY="myregistry.azurecr.io"
VERSION="0.60"

# Mirror each image
for IMAGE in "${IMAGES[@]}"; do
  docker pull ${SOURCE_REGISTRY}/${IMAGE}:${VERSION}
  docker tag ${SOURCE_REGISTRY}/${IMAGE}:${VERSION} ${TARGET_REGISTRY}/${IMAGE}:${VERSION}
  docker push ${TARGET_REGISTRY}/${IMAGE}:${VERSION}
done
```

Then install Radius using your private registry:

```console
rad install kubernetes \
  --set global.imageRegistry=myregistry.azurecr.io
```

**Note:** When using a custom registry, images are pulled directly from `<registry>/<image-name>:<tag>` format. For example, with `myregistry.azurecr.io`, the controller image will be pulled from `myregistry.azurecr.io/controller:0.60`.

### Terraform Binary Pre-downloading

By default, Radius downloads Terraform binaries at runtime when Terraform recipes are executed. You can optionally configure Radius to pre-download Terraform binaries during pod startup to improve performance.

To enable Terraform pre-downloading:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true
```

This automatically downloads the latest Terraform version. For custom sources (private repositories, proxies, etc.), specify a complete download URL:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.enabled=true \
  --set global.terraform.downloadUrl="https://my-artifactory.com/terraform_1.5.7_linux_amd64.zip"
```

#### Terraform Logging Configuration

You can configure the log level for Terraform execution to control verbosity:

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set global.terraform.loglevel="DEBUG"
```

Valid log levels are: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `OFF`. Default is `ERROR`.

### Network Policies

Radius can optionally install Kubernetes `NetworkPolicy` resources that lock down
ingress to the control-plane namespace (`radius-system`). They are **disabled by
default** and gated behind `networkPolicies.enabled`.

When enabled, three policies are applied:

- `radius-default-deny-ingress` — denies all ingress to `radius-system` pods.
- `radius-allow-internal` — re-permits east-west traffic between Radius
  components (intra-namespace), matched by the immutable
  `kubernetes.io/metadata.name` namespace label.
- `radius-allow-control-plane` — allows the Kubernetes API server to reach UCP (APIService aggregation) on port `9443`, from the CIDRs in `networkPolicies.controlPlaneCIDRs`.

Only ingress is restricted; egress is left open so UCP can reach the Kubernetes
API server and pods can resolve DNS.

> **Enforcement requires a CNI that implements NetworkPolicy** (e.g. Calico,
> Cilium, Antrea). On CNIs that do not (e.g. kindnet, flannel, the default EKS
> VPC CNI) these objects are accepted by the API server but silently **not**
> enforced.

#### Setting `controlPlaneCIDRs`

The kube-apiserver reaches UCP (APIService aggregation) over the host network, so this traffic arrives with the **node's** IP rather than a pod IP and cannot be matched by a namespace/pod selector. You must supply the source CIDR(s) via `networkPolicies.controlPlaneCIDRs` — **this is required when `networkPolicies.enabled=true`; Helm rendering fails if it is empty** — otherwise the default-deny policy would block API aggregation and break the control plane.

Use your cluster's node/control-plane subnet(s), **not** individual node IPs
(a `/32` would exclude other control-plane addresses):

- **Managed clusters (AKS/EKS/GKE):** the node pool's VPC/subnet CIDR(s).
- **kubeadm / on-prem:** the node network CIDR.
- **KinD:** the Docker network subnet, e.g.
  `docker network inspect kind -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}'`.

```console
helm upgrade --wait --install radius deploy/Chart -n radius-system \
  --set networkPolicies.enabled=true \
  --set 'networkPolicies.controlPlaneCIDRs={10.0.0.0/16}'
```

```console
# With the rad CLI
rad install kubernetes \
  --set networkPolicies.enabled=true \
  --set 'networkPolicies.controlPlaneCIDRs={10.0.0.0/16}'
```

## Verify the installation

Verify that the controller is running in the radius-system namespace:

```bash
kubectl get pods -n radius-system
```

## Uninstalling the Chart

To uninstall/delete the `radius` deployment:

```console
helm delete radius
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

Uninstalling the chart will not delete any data stored by Radius. To clean up any remaining data, delete the radius-system namespace.
