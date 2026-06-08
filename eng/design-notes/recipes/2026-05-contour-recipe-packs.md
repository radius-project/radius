# Move Contour Routing to Gateway API Recipes

* **Author**: Will Smith (@willdavsmith)

## Overview

Radius currently installs Contour by default on Kubernetes and has built-in code that renders Contour `HTTPProxy` resources for application ingress. This design moves application route rendering out of Radius core and into recipes, using Kubernetes Gateway API as the default Contour-backed path.

Radius keeps installing Contour by default for now. When Contour install is enabled, Radius also creates the shared Gateway API infrastructure used by the default route recipe:

- `GatewayClass/contour`
- `Gateway/radius` in `radius-system`
- HTTP listener on port 80 with routes allowed from application namespaces

With that infrastructure in place, the default `Radius.Compute/routes` recipe can create Gateway API route resources such as `HTTPRoute` and attach them to the shared `radius-system/radius` Gateway.

## Current Radius Behavior

Today `rad install kubernetes` installs Contour by default after installing the Radius Helm chart. The install command wires this through the existing Contour chart options:

- Helm release name: `contour`
- Namespace: `radius-system`
- Chart repository: `https://projectcontour.github.io/helm-charts`
- Default chart version: `0.1.0`
- Opt-out flag: `rad install kubernetes --skip-contour-install`

Radius also includes built-in Kubernetes rendering for Contour `HTTPProxy` resources. Gateway rendering creates a root `HTTPProxy`, route rendering creates child `HTTPProxy` resources, and the Radius RP ClusterRole includes permissions for `projectcontour.io/httpproxies`.

This change keeps default Contour installation in place, but replaces the default application routing implementation with Gateway API recipes. When users opt out with `rad install kubernetes --skip-contour-install`, Radius skips both Contour installation and the managed Contour Gateway API setup. Removing Contour from the default install remains a separate design review decision.

Radius already has a default recipe pack experience for development scenarios. `rad init --preview` creates a default recipe pack named `default` in `/planes/radius/local/resourceGroups/default` and links it to the created environment. `rad deploy` also creates or fetches that default recipe pack and injects it into environment resources that do not specify recipe packs.

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/11952

### Goals

- Keep Contour installed by default for now.
- Create the shared Contour Gateway API `Gateway` during Radius install when Contour install is enabled, with HTTP and HTTPS listeners for existing HTTPProxy behavior.
- Use the existing `Radius.Compute/routes` recipe to render Gateway API route resources by default.
- Keep Gateway API infrastructure out of the application model in the default path.
- Allow users to swap ingress behavior by changing recipe packs.

### Non goals

- Do not change the Radius application resource model.
- Do not remove Contour from the default Radius install as part of this change.
- Do not require users to define a gateway resource in application Bicep.

## User Experience

Users deploy routes with the existing `Radius.Compute/routes` resource. They do not need to define an application-level gateway resource for the default Contour path.

```bicep
resource route 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'web'
  properties: {
    application: app.id
    environment: environment
    kind: 'HTTP'
    hostnames: [
      'web.example.com'
    ]
    rules: [
      {
        matches: [
          {
            httpPath: '/'
          }
        ]
        destinationContainer: {
          resourceId: web.id
          containerName: 'web'
          containerPort: 80
        }
      }
    ]
  }
}
```

The default route recipe attaches HTTP and TLS routes to `Gateway/radius` in `radius-system`. Users who want a different Gateway API controller, such as NGINX Gateway Fabric, can select a different recipe pack or pass recipe parameters that target a different Gateway.

## Design

The default Kubernetes route path becomes:

```text
Radius install with Contour enabled -> GatewayClass/contour + Gateway/radius
Radius.Compute/containers           -> Kubernetes Deployment + Service
Radius.Compute/routes               -> Gateway API HTTPRoute/TLSRoute/TCPRoute/UDPRoute
```

The route recipe defaults are:

- `gateway_name`: `radius`
- `gateway_namespace`: `radius-system`

For HTTP and TLS routes, the route must include at least one hostname when attaching to the shared default Gateway. This prevents multiple applications from unintentionally claiming the same catch-all listener.

The Radius dynamic RP needs permission to manage Gateway API route resources:

```yaml
apiGroups:
  - gateway.networking.k8s.io
resources:
  - gateways
  - httproutes
  - tlsroutes
  - tcproutes
  - udproutes
  - referencegrants
verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
```

## Default Recipe Registration

The existing `default` recipe pack should use the Gateway API `Radius.Compute/routes` recipe. Contour installation and recipe selection are separate concerns:

- Installing Contour adds the ingress controller and Gateway API support to the cluster.
- Radius install creates the shared Contour `Gateway` only when Contour install is enabled.
- The default route recipe renders application routes that attach to that Gateway.

Today the default recipe pack follows the Radius version channel, including `latest` on the edge channel. A future hardening step should pin default recipes to the Radius release or another explicit artifact version so the default experience does not depend on floating recipe artifacts.

If Radius later stops installing Contour by default, default Gateway creation and default route recipe selection should be revisited together.

## API Design

No Radius API changes are required.

This design uses existing resource types:

- `Radius.Compute/routes@2025-08-01-preview`
- `Radius.Compute/containers@2025-08-01-preview`
- `Radius.Core/recipePacks@2025-08-01-preview`

## Implementation Details

Radius should:

- Continue installing Contour by default unless `--skip-contour-install` is set.
- Create or update the default Contour `GatewayClass` and `Gateway` after Contour installation.
- Delete the managed default `Gateway` and `GatewayClass` during uninstall.
- Grant the dynamic RP Gateway API permissions.

`resource-types-contrib` should:

- Keep the Kubernetes container recipe rendering workload and service resources.
- Use Gateway API as the default Kubernetes route recipe.
- Default the route recipe to `Gateway/radius` in `radius-system`.
- Validate that HTTP and TLS routes include hostnames when using the shared Gateway.

## Error Handling

- If Contour is not installed, the default shared Gateway is not created.
- If the recipe execution identity lacks Gateway API RBAC, route deployment fails.
- If a route has no hostname for HTTP or TLS, the default route recipe fails validation.
- If a rendered Gateway API route is invalid, e2e tests should dump the route status and gateway diagnostics.

## Test Plan

The demo validates the default recipe shape end to end:

- Contour Gateway API recipes: https://github.com/willdavsmith/radius-nginx-demo/actions/runs/26665457465
- NGINX Gateway API recipes: https://github.com/willdavsmith/radius-nginx-demo/actions/runs/26665457417

## Security

The main security consideration is Kubernetes RBAC. The recipe execution identity needs explicit permissions for Gateway API route resources.

Because the default Gateway allows routes from application namespaces, route hostnames are required for HTTP and TLS routes. This avoids accidental catch-all route attachment to the shared Gateway.

Recipe artifacts should be published from trusted locations. The local registry and module server used in the demo are test infrastructure, not a production distribution model.

## Compatibility

Keeping Contour installed by default preserves the default install experience. The application model remains stable because users continue defining containers and routes.

This changes the Kubernetes ingress implementation from Contour `HTTPProxy` to Gateway API route resources. Users who require direct HTTPProxy behavior can use an alternate recipe pack, but the default path should be Gateway API because it works with Contour today and lets users swap Gateway API controllers without Radius core changes.

## Development Plan

1. Add default Contour Gateway API infrastructure creation to Radius install and cleanup to uninstall.
2. Grant Gateway API permissions to the dynamic RP.
3. Update the default Kubernetes route recipe to attach to `radius-system/radius`.
4. Validate Contour Gateway API and NGINX Gateway API e2e paths in the demo.
5. Review default Contour installation separately.

## Alternatives Considered

### Preserve HTTPProxy as the default recipe path

This matches the current implementation more closely, but it keeps the default path tied to Contour-specific APIs. Gateway API gives Radius the same application shape while allowing alternate Gateway API controllers through recipe packs.

### Keep Contour rendering in Radius core

This preserves the current implementation but prevents users from swapping ingress behavior through recipes.

## Design Review Notes

Pending.
