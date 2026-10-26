# Use native ACA resources through a recipe pack

Proposed for team review. This decision record uses the [minimal MADR format](https://github.com/adr/madr/blob/main/template/adr-template-minimal.md); the [implementation design](2026-10-aca-implementation-design.md) contains the detailed mappings and contracts.

## Context and Problem Statement

Radius applications need to run on Azure Container Apps (ACA) using the existing containers, routes, secrets, and persistent-volume resource types. We need a resource type mapping that follows ACA's native model, which has a mapping mismatch with the network routes resource type definition.

TCP ingress is configured on the Azure Container App created by the `Radius.Compute/containers` recipe. If a user specifies a TCP `Radius.Compute/routes` resource, the recipe would have to update the container resource created by the `containers` recipe. That would make both recipes write the same Azure resource, requiring rules to prevent container updates from overwriting ingress and route deletion from deleting the app. 

## Considered Options

1. Use four Bicep recipes, with native TCP ingress configured on the container through `platformOptions`, a.k.a, "punch-through". Do not implement TCP ingress on the `routes` resource type.
2. Let the TCP route recipe update the app after the container recipe creates it.
3. Find a way for the `container` recipe to read the `routes` configuration.
4. Create a forwarding Container App for each TCP route.
## Decision Outcome

Choose **option 1**. Use the existing recipe mechanism and give each Azure resource a single recipe owner. One Container App will be created per Radius containers resource, including its normal/init containers and a per-app managed identity.

| Radius resource    | ACA implementation                                                                                                     |
|--------------------|------------------------------------------------------------------------------------------------------------------------|
| Containers         | One Container App per Radius containers resource, including its normal/init containers and a per-app managed identity. |
| HTTP routes        | Independent environment HTTP route configurations.                                                                     |
| Secrets            | Key Vault and secrets; connected apps receive scoped read access through their own identities.                         |
| Persistent volumes | Azure Files shares and environment storage definitions.                                                                |

The platform engineer supplies the ACA managed environment, deployment credentials, and infrastructure-specific parameters. Deployment credentials remain separate from workload identities. Recipes must not claim ownership of supplied infrastructure or return secret values as public outputs.

Configure TCP ingress through the container's existing `platformOptions` property, subject to the operator's enable/disable control. The container recipe writes both workload and ingress settings. External TCP requires a VNet-enabled ACA environment.

This is an **explicit deviation from the merged specification**: the ACA pack does not implement separate TCP routes. Following its ignore-and-document policy, Radius route declarations with TCP, TLS, or UDP kinds create no Azure route and return no listener. They are not automatically translated into container updates.

The alternatives are not selected because:

- **Two recipes updating one app** leave overlapping writes and deletion ownership unresolved in the current Bicep recipe contract.
- **New dependency reconciliation** requires the recipe to read additional types supplied to the recipe, which is not natively supported by Radius.
- **A forwarding app** adds a network hop, another workload, cost, and cleanup responsibility; ACA does not require it for native TCP ingress.

### Consequences

- HTTP routing, secrets, and storage remain separate resources defined on the `routes` resource type, while TCP ingresss is configured on the `containers` resource type using `platformOptions`.
- TCP configuration is platform-specific rather than portable through `Radius.Compute/routes`.
- A successful deployment can contain an unsupported route that provides **no connectivity**. Documentation must make that risk explicit, especially when moving an application between packs.
- Custom scaling must preserve the distinction between per-replica and absolute targets. ACA does not expose KEDA's general `metricType` selector; any unrepresentable promised mode remains an open conformance question.

## More Information

- [Merged functional specification](https://github.com/radius-project/radius/blob/7caf262e82d0eca944be4c0427b590e8764b713c/eng/design-notes/extensibility/2026-10-ecs-aca-core-resource-mappings.md)
- [Detailed implementation design](2026-10-aca-implementation-design.md)
- [ACA ingress model](https://learn.microsoft.com/en-us/azure/container-apps/ingress-overview)

Review should focus on the TCP portability exception, the successful-but-inactive unsupported-route behavior, and the resource/identity ownership boundaries. Acceptance criteria and file-level implementation details remain in the design rather than duplicated here.
