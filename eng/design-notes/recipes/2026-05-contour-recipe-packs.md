# Replace Built-In Contour Behavior with a Contour HTTPProxy Recipe Pack

* **Author**: Will Smith (@willdavsmith)

## Overview

Radius currently installs Contour by default on Kubernetes, and today's built-in ingress behavior uses Contour `HTTPProxy` resources. This design note documents that the same behavior can be provided by recipes instead of Radius core code.

The compatibility recipe pack uses the existing Radius resource model:

- `Radius.Compute/containers` renders the Kubernetes workload and `Service`.
- `Radius.Compute/gateways` renders the root Contour `projectcontour.io/v1 HTTPProxy` that acts as the gateway.
- `Radius.Compute/routes` renders child Contour `HTTPProxy` resources included by the root proxy.

This keeps the user-facing Radius application model stable while moving Contour-specific rendering into `resource-types-contrib`.

## Current Radius Behavior

Today `rad install kubernetes` installs Contour by default after installing the Radius Helm chart. The install command wires this through the existing Contour chart options:

- Helm release name: `contour`
- Namespace: `radius-system`
- Chart repository: `https://projectcontour.github.io/helm-charts`
- Default chart version: `0.1.0`
- Opt-out flag: `rad install kubernetes --skip-contour-install`

Radius also includes built-in Kubernetes rendering for Contour `HTTPProxy` resources. The gateway renderer creates the root `HTTPProxy` that acts as the gateway, and route rendering creates child `HTTPProxy` resources included by that root proxy. The Radius RP ClusterRole already includes permissions for `projectcontour.io/httpproxies`.

This design does not change the default install behavior. It only moves the Contour-specific application rendering path into recipes. Whether Radius should stop installing Contour by default should be reviewed separately.

Radius already has default recipe pack behavior for development scenarios. `rad init --preview` creates a default recipe pack named `default` in `/planes/radius/local/resourceGroups/default`, and links it to the created environment. `rad deploy` also creates or fetches that default recipe pack and injects it into environment resources that do not specify recipe packs. The current default pack includes core Kubernetes recipes such as containers, persistent volumes, routes, and secrets.

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/11952

### Goals

- Mirror today's Contour `HTTPProxy` behavior with a contributed recipe pack.
- Keep applications using `Radius.Compute/gateways`, `Radius.Compute/routes`, and `Radius.Compute/containers`.
- Move Contour-specific rendering out of Radius core.
- Show that users can swap to Gateway API by changing recipe packs.
- Keep removal of default Contour installation as a separate design review decision.

### Non goals

- Do not change the Radius application resource model.
- Do not require users to move to Gateway API.
- Do not remove Contour from the default Radius install as part of this change.

## User Experience

Users continue deploying the same application shape. The selected recipe pack determines whether Radius renders Contour HTTPProxy resources or Gateway API resources.

```bash
rad deploy contour-httpproxy-recipe-pack.bicep --group default -e default
rad env update default --recipe-packs contour-httpproxy-pack --preview
rad deploy app.bicep --application contour-httpproxy-demo -e default
```

To use Gateway API instead, users attach a different recipe pack:

```bash
rad env update default --recipe-packs contour-gateway-api-pack --preview
rad deploy app.bicep --application contour-gateway-api-demo -e default -p gatewayClassName=contour
```

The application Bicep does not need provider-specific logic for this swap.

## Design

The Contour HTTPProxy recipe pack provides parity with today's implementation:

```text
Radius.Compute/containers -> Kubernetes Deployment + Service
Radius.Compute/gateways   -> root Contour HTTPProxy
Radius.Compute/routes     -> child Contour HTTPProxy resources
```

The gateway recipe maps Radius gateway properties to the root `HTTPProxy`:

- gateway hostname -> `HTTPProxy.spec.virtualhost.fqdn`
- gateway TLS settings -> `HTTPProxy.spec.virtualhost.tls`
- route references -> `HTTPProxy.spec.includes[]`

The route recipe maps Radius route properties to child `HTTPProxy` resources:

- `rules[].matches[].httpPath` -> `HTTPProxy.spec.routes[].conditions[].prefix`
- `rules[].destinationContainer` -> `HTTPProxy.spec.routes[].services[]`
- recipe parameter `hostname` or route hostnames -> `HTTPProxy.spec.virtualhost.fqdn`

The recipe execution identity needs permission to manage `HTTPProxy` resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: radius-contour-httpproxy-recipes
rules:
  - apiGroups:
      - projectcontour.io
    resources:
      - httpproxies
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - patch
      - delete
```

Gateway API remains available as a separate recipe pack:

```text
Radius.Compute/gateways -> Gateway API Gateway
Radius.Compute/routes   -> Gateway API HTTPRoute
Radius.Compute/containers -> Kubernetes Deployment + Service
```

This lets users switch from Contour HTTPProxy to Contour Gateway API, NGINX Gateway Fabric, or another Gateway API controller without Radius core changes.

## Default Recipe Registration

Radius already has a default recipe pack experience. The question for Contour is how that existing default should include HTTPProxy parity while Contour remains installed by default.

Contour installation and recipe-pack selection are separate decisions:

- Installing Contour adds the ingress controller and its CRDs to the cluster.
- Selecting Contour HTTPProxy recipes makes the default Radius application model resolve to Contour-backed Kubernetes resources.

While `rad install kubernetes` continues installing Contour by default, the default recipe pack experience should include the Contour HTTPProxy gateway and route behavior. This preserves today's default experience while allowing Radius core to stop rendering Contour resources directly.

The default environment setup should be:

```text
rad init --preview or rad deploy with an environment
  -> creates or fetches the default recipe pack
  -> includes Contour HTTPProxy recipes while Contour is the default ingress controller
  -> attaches the default pack to the environment when no explicit recipe packs are set
```

There are two implementation options:

- Extend the existing `default` recipe pack with Contour HTTPProxy gateway and route recipes.
- Keep ingress recipes in a separate default-attached pack.

The default recipes should be pinned to the Radius release or another explicit artifact version. The default experience should not depend on floating latest recipe artifacts.

If Radius later stops installing Contour by default, default Contour recipe selection should be revisited at the same time. A default HTTPProxy route recipe without Contour installed would make the default environment fail for gateway and route deployments.

## API design

No Radius API changes are required.

This design uses existing resource types:

- `Radius.Compute/gateways@2025-08-01-preview`
- `Radius.Compute/routes@2025-08-01-preview`
- `Radius.Compute/containers@2025-08-01-preview`
- `Radius.Core/recipePacks@2025-08-01-preview`

## Implementation Details

`resource-types-contrib` should provide:

- A container recipe that renders Kubernetes workload and service resources.
- A Contour HTTPProxy route recipe.
- A Contour HTTPProxy gateway recipe that renders the root proxy.
- Gateway API gateway and route recipes as an alternate pack.

The existing default recipe pack flow should include Contour HTTPProxy gateway and route behavior while Contour remains installed by default. Contour installation can remain part of the default Radius install while this capability is introduced. Removing Contour from the default install should be reviewed separately because it changes installation behavior for existing users.

## Error Handling

- If Contour is not installed, the HTTPProxy recipe fails because the `projectcontour.io` API group is unavailable.
- If the recipe execution identity lacks RBAC for `httpproxies`, recipe deployment fails.
- If the rendered HTTPProxy is invalid, tests should wait for `HTTPProxy.status.currentStatus=valid` and dump Kubernetes diagnostics on timeout.

## Test plan

The demo validates the capability with three end-to-end paths:

- Contour HTTPProxy recipes: https://github.com/willdavsmith/radius-nginx-demo/actions/runs/26118318051
- Contour Gateway API recipes: https://github.com/willdavsmith/radius-nginx-demo/actions/runs/26118318008
- NGINX Gateway API recipes: https://github.com/willdavsmith/radius-nginx-demo/actions/runs/26118318007

The HTTPProxy test verifies that Radius can deploy the same app model through recipes and receive traffic through Contour Envoy.

## Security

The main security consideration is Kubernetes RBAC. The recipe execution identity needs explicit permissions for `projectcontour.io/httpproxies`.

Recipe artifacts should be published from trusted locations. The local registry and module server used in the demo are test infrastructure, not a production distribution model.

## Compatibility

The HTTPProxy recipe pack preserves compatibility with today's Contour behavior.

Keeping Contour installed by default and including Contour HTTPProxy behavior in the default recipe pack experience initially preserves install and behavior compatibility. If default Contour installation is removed later, migration guidance should tell users to either install Contour and attach the HTTPProxy recipe pack, or install a Gateway API controller and attach a Gateway API recipe pack.

## Development plan

1. Add the Contour HTTPProxy recipe pack to `resource-types-contrib`.
2. Document the required HTTPProxy RBAC.
3. Include Contour HTTPProxy behavior in the existing default recipe pack experience while Radius installs Contour by default.
4. Keep e2e coverage for HTTPProxy, Contour Gateway API, and NGINX Gateway API recipe packs.
5. Document how to switch between HTTPProxy and Gateway API recipe packs.
6. Review default Contour installation separately.

## Alternatives considered

### Move directly to Gateway API recipes

Gateway API is portable and should be available as a recipe pack, but it does not exactly match today's Contour HTTPProxy implementation.

### Keep Contour rendering in Radius core

This preserves the current implementation but prevents users from swapping ingress behavior through recipes.

## Design Review Notes

Pending.
