# Use Secret references for GitOps deployment parameters

## Status

Proposed, 2026-11-04.

Issues: [#13500](https://github.com/radius-project/radius/issues/13500) and [#12999](https://github.com/radius-project/radius/issues/12999).

[Technical design](https://gist.github.com/willdavsmith/6fb41f2374680d7e6ea05c452f61e822): API details, authorization, controller behavior, tests, and implementation questions.

## Context

Users configure Radius Flux deployments in `radius-gitops-config.yaml` at the root of their watched Git repository. The `params` field selects a `.bicepparam` file containing deployment inputs.

Today, Radius copies those values into a Kubernetes DeploymentTemplate, including parameters marked `@secure()`. Anyone who can read that object can read the supplied credentials.

Flux and Argo CD users already have tools such as SOPS, External Secrets Operator, and Sealed Secrets to provision Kubernetes Secrets. Radius should consume those Secrets rather than require credentials in parameter files.

## Decision

Add optional Secret references alongside existing file-based parameters. Resolve them in the Radius deployment controller when deploying, not while generating manifests.

### User workflow

1. Provision a Kubernetes Secret using an existing secret-management tool.
2. Reference its name and key in `radius-gitops-config.yaml`:

```yaml
config:
  - name: app.bicep
    params: app.bicepparam
    namespace: my-app
    parameterReferences:
      databasePassword:
        secretKeyRef:
          name: database-credentials
          key: password
```

Keep ordinary values in `.bicepparam` and omit the referenced secret. When combining a parameter file with references, use Bicep's [`using none`](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/bicep-using#the-using-none-statement) declaration; Radius validates the combined inputs.

The template still declares `databasePassword` as a secure string. Users edit their source configuration, not the generated DeploymentTemplate.

### Expected behavior

- References select Secrets in the deployment's namespace. Operators control access through standard Kubernetes service accounts and RBAC.
- Generated manifests and deployment objects retain references, not resolved values. Secret values must also stay out of logs, events, and diagnostics.
- Secret updates trigger reconciliation without a Git change.
- Missing, invalid, or inaccessible inputs fail visibly before a new deployment. Radius does not fall back to another value.
- Deleting the application does not delete externally managed Secrets.

Flux and Argo CD use the same Radius resolution path. Neither integration requires installing the other.

### Compatibility

Existing `params` configurations continue to work unchanged. Adoption is optional and per parameter; there is no required migration.

Users adopting references remove the corresponding inline value and template default. Previously exposed credentials should be rotated. Editing the configuration does not remove historical copies.

### Review decision

**Define which namespaces each Git source may deploy into before enabling tenant use.** A repository must not gain access to another namespace's Secrets by changing its destination. Secret-read RBAC alone does not enforce this source boundary.

## Consequences

Users can keep their existing secret-management tools and stop passing credentials through deployment manifests. Platform operators must configure namespace access and Secret-read permissions.

Deployment authors in a namespace share its approved Secret access; use separate namespaces where different access is required. Templates remain trusted to use the credentials they receive.

The initial proposal supports secure string inputs, not secure objects. Existing inline values remain exposed until users replace them with references.

AI-drafted; proposed behavior is not yet implemented. Implementation constraints and verification results are retained in the technical design.
