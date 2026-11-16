# Use Secret references for GitOps deployment parameters

Issues: [#13500](https://github.com/radius-project/radius/issues/13500) and [#12999](https://github.com/radius-project/radius/issues/12999).

[Technical design](https://gist.github.com/willdavsmith/6fb41f2374680d7e6ea05c452f61e822): API details, authorization, controller behavior, tests, and implementation questions.

## Context

Users configure Radius Flux deployments in `radius-gitops-config.yaml` at the root of their watched Git repository. The `params` field selects a `.bicepparam` file containing deployment inputs.

Today, Radius copies those values into a Kubernetes DeploymentTemplate, including parameters marked `@secure()`. Anyone who can read that object can read the supplied credentials.

Flux and Argo CD users already have tools such as SOPS, External Secrets Operator, and Sealed Secrets to provision Kubernetes Secrets. Radius should consume those Secrets rather than require credentials in parameter files.

## Decision

Declare Secret references in `.bicepparam` using Bicep's `externalInput()` function. Resolve them in the Radius deployment controller when deploying, not while generating manifests.

### User workflow

1. Provision a Kubernetes Secret using an existing secret-management tool.
2. Keep `radius-gitops-config.yaml` selecting the template, parameter file, and destination:

   ```yaml
   config:
     - name: app.bicep
       params: app.bicepparam
       namespace: my-app
   ```

3. Keep ordinary values and Secret references together in `app.bicepparam`:

   ```bicep
   using './app.bicep'

   param applicationName = 'orders'
   param databasePassword = externalInput('radius.secretKeyRef', {
     name: 'database-credentials'
     key: 'password'
   })
   ```

[`externalInput()`](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/bicep-functions-parameters-file#externalinput) describes an input supplied by an external tool after compilation. `radius.secretKeyRef` is the Radius input kind for Secret references.

The template still declares `databasePassword` as a secure string. The linked parameter file retains Bicep completion, hover information, and validation of known inputs. Radius checks the resolved credential before submission.

### Expected behavior

- References select Secrets in the deployment's namespace. Operators provision a `radius-deployer` service account and scoped RBAC; the controller impersonates that account to read approved Secrets.
- Generated manifests and deployment objects retain references, not resolved values. Secret values must also stay out of logs, events, and diagnostics.
- Radius polls referenced Secrets without a Git change, using a 60-second baseline with up to 20% jitter rather than Secret watches. This avoids Secret list/watch permissions. The interval is not a rotation deadline: active deployments, retries, and controller load can delay application.
- Missing, invalid, or inaccessible inputs fail visibly before a new deployment. Radius does not fall back to another value.
- The initial version accepts direct Secret bindings, not expressions that combine or transform them.
- Deleting the application does not delete externally managed Secrets.

Flux and Argo CD use the same Radius resolution path. Neither integration requires installing the other.

### Compatibility

Existing `params` configurations continue to work unchanged. Adoption is optional and per parameter; there is no required migration.

Users adopting references replace the corresponding literal assignment with `externalInput()` and remove any template default. This requires Bicep 0.37.4 or later; the repository pins 0.46.1. Previously exposed credentials should be rotated. Editing the configuration does not remove historical copies.

Upgrade the CRDs and controller together before using references. Mixed controller versions and downgrades with reference-bound deployments are unsupported; migrate those deployments before downgrading.

### Trust model

Repository authors and deployment templates must be trusted. Radius automatically processes watched GitRepository objects containing `radius-gitops-config.yaml`, so registering or modifying a source grants deployment authority.

Centralized sources remain supported: a GitRepository in `flux-system` may deploy into another namespace. Secret-read RBAC limits the destination reader's access but does not authorize sources or provide hostile multi-tenant isolation. Operators must control sources, destination namespaces, service accounts, and RBAC.

### Release requirements

Verify credential handling across deployment-engine persistence, providers, and diagnostics before release. Validate end-to-end secret provisioning and Argo CD sync, multi-replica recovery, and production load. Provide operator-facing polling configuration and recovery guidance.

## Consequences

Users can keep their existing secret-management tools and stop passing credentials through deployment manifests. Platform operators must configure namespace access and Secret-read permissions.

Deployment authors in a namespace share its approved Secret access; use separate namespaces where different access is required. Templates remain trusted to use the credentials they receive.

The initial version supports secure string inputs, not secure objects. Existing inline values remain exposed until users replace them with references.

AI-drafted. Implementation constraints and verification requirements are retained in the technical design.
