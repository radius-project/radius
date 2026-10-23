# Topic: Run Radius core resources on Amazon ECS and Azure Container Apps

- **Author**: Reshma Abdulrahim (@Reshrahim)
- **Product specification for**: [issue #13013](https://github.com/radius-project/radius/issues/13013)

## Topic Summary

Radius core resource types exist so a developer can describe an application once and a platform engineer can decide where it runs. There are four: `Radius.Compute/containers`, `Radius.Compute/routes`, `Radius.Compute/persistentVolumes`, and `Radius.Security/secrets`.

Today that promise only holds for Kubernetes. It is the only platform with a complete recipe pack, and the container schema is, in the words of its own README, "heavily biased towards Kubernetes Pods and Deployments."

This spec defines what is required to make it real on **Azure Container Apps (ACA)** and **Amazon ECS on Fargate**: two widely adopted serverless container platforms, both named as intended targets in the container type's own documentation.

### At a glance

|                          | Azure Container Apps      | Amazon ECS on Fargate                                                                                               |
|--------------------------|---------------------------|---------------------------------------------------------------------------------------------------------------------|
| Platform engineer brings | A managed environment     | Cluster, VPC, subnets, security groups, execution role, ALB, Cloud Map namespace, EFS file system and mount targets |
| Containers become        | Container Apps            | Task definitions + services                                                                                         |
| Routes become            | `httpRouteConfigs` rules  | ALB listener rules                                                                                                  |
| Volumes become           | Azure Files share         | EFS access point                                                                                                    |
| Secrets become           | Key Vault secret          | Secrets Manager secret                                                                                              |
| Service discovery        | Environment-internal FQDN | Service Connect alias                                                                                               |

### Top level goals

1. **Ship recipe packs for ACA and ECS** covering `containers`, `routes`, `persistentVolumes`, and `secrets`, contributed to `resource-types-contrib` next to the existing `kubernetes`, `azure-aks`, and `azure-aci` packs, so an application is portable across Kubernetes, ACA, and ECS.
2. **Document property and platform support for each type.** Publish per-property, per-platform support documentation with every pack in `resource-types-contrib`, and a platform page on `docs.radapp.io`, so both a developer and a platform engineer know what they get before deploying.
3. **Create a reference application for ACA and ECS** that exercises all four core types on each platform.

### Non-goals (out of scope)

- **Other container platforms.** Google Cloud Run and Nomad.
- **Live migration between platforms.** Moving a running workload from Kubernetes to ACA is a redeploy.
- **100% property parity.** A property a platform cannot express is marked unsupported and documented as such.
- **New core-type properties for single-platform features.** ACA traffic splitting, ECS capacity providers, Fargate Spot. These stay reachable through `platformOptions`.
- **Deployment-time validation.** Radius has no mechanism to reject an unsupported property before provisioning, so documentation is the only way a user learns what a platform will do with it.
- **Non-core resource types.** Databases, caches, and messaging already have their own recipes.

## User profile and challenges

### User persona(s) and the challenges they face

**Primary — Priya, platform engineer.** Owns the Radius environments her organization deploys into. Her company standardized on ACA (or ECS) deliberately. She works in two modes:

- *Consuming a pack* — she registers a published `azure-aca` or `aws-ecs` pack, points an environment at infrastructure she already runs, and hands it to her developers.
- *Authoring or customizing a pack* — the published pack does not match her organization's requirements, so she forks `resource-types-contrib` and builds her own recipes.

She cannot deploy to either platform today. Adopting Radius leaves her two options — move her applications onto Kubernetes, a platform her team deliberately chose not to operate, or build and maintain an ACA or ECS recipe pack herself with no documented contract for what each core type must guarantee.

**Primary — Dev, application developer.** Writes the Bicep that defines his application — a container, a route, a volume, a secret — and deploys it to the target environment.

With no recipe pack for ACA or ECS, there is no environment for him to target at all.

### Positive user outcome

Choosing a container platform stops being a choice about whether to use Radius. Priya gives her developers an environment on the platform her organization already runs, Dev deploys to it, and what each property does on each platform is written down before anyone deploys.

## Key scenarios

### Scenario 1: Run Radius on ACA

Priya registers the `azure-aca` pack against her existing ACA managed environment.

Dev deploys an application using the core types — two containers, an HTTP route, a volume, and a secret. It becomes Container Apps, ACA ingress, an Azure Files share, and a Key Vault reference, and connections resolve to the environment-internal FQDN.

### Scenario 2: Run Radius on ECS

Priya registers the `aws-ecs` pack against her existing Fargate cluster, VPC, ALB, Cloud Map namespace, and EFS file system. Radius deploys into that infrastructure; it does not create it.

Dev deploys the same application definition. It becomes task definitions and services, an ALB target group and listener rule, an EFS access point, and a Secrets Manager secret, and connections resolve to a Service Connect alias.

## Key dependencies and risks

**Dependency — `resource-types-contrib`.** Both packs live there, alongside `kubernetes`, `azure-aks`, and `azure-aci`. The ACI pack is the closest precedent for how a non-Kubernetes mapping is scoped, documented, and shipped.

**Dependency — platform infrastructure must pre-exist.** Neither pack creates an ACA managed environment or an ECS cluster/VPC/ALB. Priya brings those and passes them as `recipeParameters`, as the Kubernetes routes recipe already requires a pre-existing gateway.

**Risk — the schema's Kubernetes bias means fewer properties map than users expect.** Requests vs. limits, `exec` probes, `restartPolicy`, and secret volumes all assume Kubernetes semantics. *Mitigation:* publish the support documentation up front so every unsupported property is a documented decision, and keep `platformOptions` as the sanctioned way to reach platform-specific settings.

**Risk — silent divergence between packs.** The two packs share no code. ACA is Bicep, ECS is Terraform, written by different contributors and tested on separate paths. Two independently authored packs can map the same property differently — for example, whether `resources.requests` rounds up or fails when the value is not a legal platform combination.

*Mitigation:* the published documentation is the written contract, and the reference applications (Feature 3) exercise each pack against it.

## Key assumptions to test and questions to answer

**Q1 — Do ACA and ECS need new resource types, or do the existing four work?**

The existing four. All of them map with meaningful coverage, and new types would fragment the portability promise that is the entire point of the feature.

*Validated during implementation:* the Feature 1 and Feature 2 packs, and the reference application for each platform.

**Q2 — When a recipe cannot honor a property, does it fail the deployment or ignore the property?**

Ignore it and document it. Failing on a value Radius itself accepts would break portability in practice.

*Validated during implementation:* prototyping both container recipes with a property neither platform supports, and confirming the deployment succeeds.

**Q3 — What happens when a schema marks a property required but a platform has no equivalent?**

A required property forces every application to set a value even when the target platform has no equivalent and will ignore it. Required properties should be revisited with the core type owners as platforms are added, and a property that is not universally meaningful should become optional. Until then the recipe ignores what it cannot honor.

*Validated during implementation:* enumerating every required property with no equivalent on a target platform, making each one optional in a draft schema, and confirming the existing recipes still provision without it.

## Current state

Three recipe packs ship in [`resource-types-contrib`](https://github.com/radius-project/resource-types-contrib). One of them, `azure-aci`, already targets a non-Kubernetes compute platform and is the working precedent for this spec.

| Pack         | Compute platform          | Core types covered                           |
|--------------|---------------------------|----------------------------------------------|
| `kubernetes` | Kubernetes                | All four                                     |
| `azure-aks`  | Kubernetes on AKS         | All four                                     |
| `azure-aci`  | Azure Container Instances | `containers`, `persistentVolumes`, `secrets` |

The ACI pack proved a non-Kubernetes platform can be mapped, and set the convention for documenting it: it records where ACI semantics differ as `Note:` lines in `Compute/containers/README.md`, covering `daprSidecar`, `replicas`, `autoScaling.*`, `args`, and `workingDir`.

Neither ACA nor ECS has a pack today. No recipe exists for either platform for any of the four core types.

Two Azure recipes do exist that the ACA pack should reuse rather than rewrite:

- [`azure-file-volumes.bicep`](https://github.com/radius-project/resource-types-contrib/blob/main/Compute/persistentVolumes/recipes/azure/bicep/azure-file-volumes.bicep) — provisions an Azure Files share.
- [`azure-keyvault-secrets.bicep`](https://github.com/radius-project/resource-types-contrib/blob/main/Security/secrets/recipes/azure/bicep/azure-keyvault-secrets.bicep) — provisions a Key Vault secret.

Both provision Azure resources that are independent of the compute platform. Both are listed today as Azure (ACI) and emit outputs shaped for ACI, so implementation confirms what ACA needs from them. ECS has no equivalent to reuse.

## Desired user experience outcome

*As Priya, consuming a pack:* I register the published `azure-aca` or `aws-ecs` pack, point an environment at the ACA managed environment or ECS cluster I already run, and hand it to my developers. I never write a recipe, and I read the pack's documentation first so I know what my platform can and cannot do before I commit to it.

*As Dev:* I write one application definition — containers, a route, a volume, a secret — and deploy it to whichever environment I am given, ACA or ECS. When a property is unsupported on a platform, the documentation tells me before I deploy, not after.

*As Priya, authoring or customizing a pack:* I do not re-derive each decision myself. The documentation tells me, property by property, what the contract is on each platform and the reference application for my platform proves my pack honors it.

### Detailed user experience

> **Note:** The scenarios below are a proposal that gives direction, not a fully prescribed design. Parameter names can change during implementation.

#### Scenario 1 — Run Radius on Azure Container Apps

**Step 1 — Priya registers the recipe pack.** Priya references the published `azure-aca` pack, which maps all four core types onto ACA resources.

```bicep
resource acaPack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-aca'
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/azure-aca-recipes/containers:latest'
      }
      'Radius.Compute/routes': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/azure-aca-recipes/routes:latest'
      }
      'Radius.Compute/persistentVolumes': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/azure-aca-recipes/persistentvolumes:latest'
      }
      'Radius.Security/secrets': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/azure-aca-recipes/secrets:latest'
      }
    }
  }
}
```

**Step 2 — Priya creates the environment.** Azure subscription and resource group go through `providers.azure`; the pre-existing ACA managed environment goes through `recipeParameters`, following the Kubernetes routes recipe precedent (`gatewayName`, `gatewayNamespace`).

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'aca-prod'
  properties: {
    recipePacks: [acaPack.id]
    providers: {
      azure: {
        subscriptionId: '00000000-0000-0000-0000-000000000000'
        resourceGroupName: 'rg-radius-prod'
      }
    }
    recipeParameters: {
      'Radius.Compute/containers': {
        acaEnvironment: '/subscriptions/<sub-id>/resourceGroups/rg-radius-prod/providers/Microsoft.App/managedEnvironments/aca-prod'
        workloadProfileName: 'Consumption'
      }
      'Radius.Compute/routes': {
        acaEnvironment: '/subscriptions/<sub-id>/resourceGroups/rg-radius-prod/providers/Microsoft.App/managedEnvironments/aca-prod'
      }
      'Radius.Compute/persistentVolumes': {
        acaEnvironment: '/subscriptions/<sub-id>/resourceGroups/rg-radius-prod/providers/Microsoft.App/managedEnvironments/aca-prod'
      }
    }
  }
}
```

**Step 3 — Dev writes the application.** It is unchanged from what he runs on Kubernetes.

```bicep
resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'storefront'
  properties: { environment: env.id }
}

resource dbPassword 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'db-password'
  properties: {
    application: app.id
    environment: env.id
    kind: 'generic'
    data: { password: { value: dbPasswordValue } }
  }
}

resource uploads 'Radius.Compute/persistentVolumes@2025-08-01-preview' = {
  name: 'uploads'
  properties: {
    application: app.id
    environment: env.id
    sizeInGib: 50
    allowedAccessModes: 'ReadWriteMany'
  }
}

resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    environment: env.id
    containers: {
      web: {
        image: 'ghcr.io/contoso/storefront:1.4.2'
        ports: { http: { containerPort: 8080, protocol: 'TCP' } }
        resources: { requests: { cpu: '0.5', memoryInMib: 1024 } }
        readinessProbe: { httpGet: { path: '/healthz', port: 8080 } }
        volumeMounts: [{ volumeName: 'uploads', mountPath: '/var/uploads' }]
      }
    }
    volumes: { uploads: { persistentVolume: { resourceId: uploads.id, accessMode: 'ReadWriteMany' } } }
    connections: { secrets: { source: dbPassword.id } }
    replicas: 2
  }
}

resource route 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'storefront-route'
  properties: {
    application: app.id
    environment: env.id
    kind: 'HTTP'
    hostnames: ['storefront.contoso.com']
    rules: [{
      matches: [{ httpPath: '/' }]
      destinationContainer: { resourceId: frontend.id, containerName: 'web', containerPort: 8080 }
    }]
  }
}
```

**Step 4 — Dev deploys.** `rad deploy app.bicep` against the ACA environment. It becomes Container Apps, an ACA HTTP route, an Azure Files share, and a Key Vault secret.

**Step 5 — Dev reaches for a property ACA does not support.** He wants an `exec` liveness probe. The developer documentation records it as unsupported, gives the reason, points him at `httpGet` and `tcpSocket`, and states that the recipe ignores the property if he writes it anyway. He uses `httpGet`.

#### Scenario 2 — Run Radius on Amazon ECS

**Step 1 — Priya registers the recipe pack.** Same shape as Scenario 1, but the ECS recipes are Terraform rather than Bicep.

```bicep
resource ecsPack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'aws-ecs'
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'terraform'
        source: 'git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/aws/terraform'
      }
      'Radius.Compute/routes': {
        kind: 'terraform'
        source: 'git::https://github.com/radius-project/resource-types-contrib.git//Compute/routes/recipes/aws/terraform'
      }
      'Radius.Compute/persistentVolumes': {
        kind: 'terraform'
        source: 'git::https://github.com/radius-project/resource-types-contrib.git//Compute/persistentVolumes/recipes/aws/terraform'
      }
      'Radius.Security/secrets': {
        kind: 'terraform'
        source: 'git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/recipes/aws/terraform'
      }
    }
  }
}
```

> **Note:** Terraform is an assumption here, if a different IaC tool becomes the better fit for AWS, the pack switches to it.

**Step 2 — Priya creates the environment.** ACA bundles networking, DNS, and logging into the single managed environment resource. ECS has no such container, so each piece is a parameter:

| Parameter                 | Why ECS needs it                                                                               | ACA equivalent                                                                                                                                                                   |
|---------------------------|------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `clusterArn`              | An ECS service must belong to a cluster; there is no implicit default.                         | The managed environment.                                                                                                                                                         |
| `subnetIds`               | Fargate's `awsvpc` network mode gives each task an ENI, which must be placed in named subnets. | The managed environment owns its subnet.                                                                                                                                         |
| `securityGroupIds`        | That ENI needs a security group. AWS has no default that allows traffic between tasks.         | The managed environment allows internal traffic.                                                                                                                                 |
| `executionRoleArn`        | The ECS agent assumes this role to pull the image from ECR and write logs to CloudWatch.       | Logs: handled by the managed environment. Image pulls: configured on each Container App, not the environment, using registry credentials or a managed identity with pull access. |
| `serviceConnectNamespace` | A Cloud Map namespace is what gives containers DNS names for `connections` and `hosts`.        | `*.internal.<env>.<region>.azurecontainerapps.io`.                                                                                                                               |
| `fileSystemId`            | The application access point must belong to an EFS file system with mount targets in the VPC.  | The recipe creates an Azure Files share.                                                                                                                                         |

A recipe could create some of these, but subnets, security groups, and IAM roles are governed by Priya's existing AWS controls, so v1 accepts them as inputs.

```bicep
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'ecs-prod'
  properties: {
    recipePacks: [ecsPack.id]
    providers: {
      aws: {
        accountId: '123456789012'
        region: 'us-west-2'
      }
    }
    recipeParameters: {
      'Radius.Compute/containers': {
        clusterArn: 'arn:aws:ecs:us-west-2:123456789012:cluster/radius-prod'
        subnetIds: ['subnet-0a1b2c3d', 'subnet-4e5f6a7b']
        securityGroupIds: ['sg-0123456789abcdef0']
        executionRoleArn: 'arn:aws:iam::123456789012:role/ecsTaskExecutionRole'
        serviceConnectNamespace: 'radius-prod.internal'
      }
      'Radius.Compute/routes': {
        loadBalancerArn: 'arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/radius-prod/abc123'
        listenerArn: 'arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/radius-prod/abc123/def456'
        vpcId: 'vpc-0abc123'
      }
      'Radius.Compute/persistentVolumes': {
        fileSystemId: 'fs-0123456789abcdef0'
      }
    }
  }
}
```

**Step 3 — Dev deploys the same application.** It becomes task definitions and ECS services, an ALB target group and listener rule, an access point in Priya's EFS file system, and a Secrets Manager secret. The source is identical; the behavior is not, and the documentation says where.

## Key investments

### Core type mapping

Each pack implements the same four core types. This is the type-level contract; **Property mapping** below refines it property by property.

| Core type                          | Kubernetes                                  | Azure Container Apps                                                               | Amazon ECS (Fargate)                                                              |
|------------------------------------|---------------------------------------------|------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------|
| `Radius.Compute/containers`        | Deployment + Service                        | `Microsoft.App/containerApps` in the managed environment                           | Task definition + ECS service in the cluster                                      |
| `Radius.Compute/routes`            | Gateway API `HTTPRoute` on a shared Gateway | `httpRouteConfigs` path rule on the managed environment                            | ALB listener rule → target group → service                                        |
| `Radius.Compute/persistentVolumes` | PersistentVolumeClaim                       | Azure Files share + environment storage definition, mounted via `template.volumes` | EFS file system access point, mounted via task `volumes[].efsVolumeConfiguration` |
| `Radius.Security/secrets`          | Kubernetes `Secret`                         | Key Vault secret, referenced by `keyVaultUrl` + managed identity                   | Secrets Manager secret, referenced by ARN in `secrets[].valueFrom`                |

What each pack **consumes** versus **creates** differs. The consumed infrastructure is what Priya brings and passes as `recipeParameters`:

| Pack        | Consumes (pre-existing, supplied as recipe parameters)                                                              | Creates                                                                                        |
|-------------|---------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `azure-aca` | Managed environment, workload profile                                                                               | Container Apps, storage definitions, `httpRouteConfigs` rules, Key Vault and the secrets in it |
| `aws-ecs`   | Cluster, VPC, subnets, security groups, execution role, ALB, Cloud Map namespace, EFS file system and mount targets | Task definitions, services, target groups, listener rules, EFS access points, secrets          |

Secrets also differ. The ACA pack creates the Key Vault it writes into, following the existing Azure secrets recipe, so Priya brings nothing; ECS needs no equivalent because Secrets Manager is account-level.

Everything in the **Creates** column is owned by the Radius resource that produced it and is deleted with it. Everything in **Consumes** outlives the application — deleting an application never touches Priya's managed environment, cluster, ALB, EFS file system, mount targets, or stored data.

These lists are not final — implementation may identify further infrastructure each pack has to consume or create.

### Property mapping

What each core-type property becomes on each platform. There is no Kubernetes column — the core schema was designed against Kubernetes, so every property below is supported there.

Two things to know before reading the tables:

- **The mappings are design intent.** No ACA or ECS recipe exists yet, so implementation validates every cell and corrects it where it is wrong.
- **Every mapping is documented twice.** Inline in the recipe where the property is handled or ignored, and in the per-type and per-pack READMEs `resource-types-contrib` already uses.

The two states are:

- **Supported** — the recipe honors the property. A different target field, or a value the platform rounds to a legal one, still counts as supported. Where behavior differs, a note under the table says how.
- **Unsupported** — the platform cannot deliver what the property promises, so the recipe ignores it. Setting it never fails a deployment.

Where a platform honors part of a property and not the rest, that part gets its own row rather than being averaged into a single state.

#### Containers

| Property                                    | Azure Container Apps                                   | Amazon ECS (Fargate)                                     |
|---------------------------------------------|--------------------------------------------------------|----------------------------------------------------------|
| `image`                                     | Supported — `template.containers[].image`              | Supported — `containerDefinitions[].image`               |
| `command` / `args`                          | Supported — `command` / `args`                         | Supported — `entryPoint` / `command`                     |
| `env.*.value`                               | Supported — `env[].value`                              | Supported — `environment[]`                              |
| `env.*.valueFrom.secretKeyRef`              | Supported — app secret + `env[].secretRef`             | Supported — `secrets[].valueFrom`                        |
| `workingDir`                                | **Unsupported** — no equivalent                        | Supported — `workingDirectory`                           |
| `containers.*.initContainer`                | Supported — `template.initContainers[]`                | Supported — `dependsOn: SUCCESS` + `essential: false`    |
| `ports.*.containerPort`                     | Supported — `ingress.targetPort`                       | Supported — `portMappings[].containerPort`               |
| `ports.*.protocol: UDP`                     | **Unsupported** — ingress is HTTP/TCP only             | **Unsupported** — needs an NLB                           |
| `resources.requests`                        | Supported — `resources.cpu` / `memory`                 | Supported — task `cpu` / `memory`                        |
| `resources.limits`                          | **Unsupported** — no separate limit exists             | Supported — container `memory`                           |
| `livenessProbe.exec`                        | **Unsupported** — HTTP/TCP probes only                 | Supported — `healthCheck.command`                        |
| `livenessProbe.httpGet` / `tcpSocket`       | Supported — `probes[]` type `Liveness`                 | **Unsupported** — health checks are command-based        |
| `readinessProbe.httpGet`                    | Supported — HTTP readiness probe                       | Supported — ALB target group health check                |
| `readinessProbe.tcpSocket`                  | Supported — TCP readiness probe                        | **Unsupported** — ALB does not provide TCP checks        |
| `readinessProbe.exec`                       | **Unsupported** — HTTP/TCP probes only                 | **Unsupported** — ALB cannot run container commands      |
| `restartPolicy`                             | **Unsupported** — always restarts; not configurable    | **Unsupported** — boolean toggle only                    |
| `replicas`                                  | Supported — `scale.minReplicas` / `maxReplicas`        | Supported — service `desiredCount`                       |
| `autoScaling.maxReplicas`                   | Supported — `scale.maxReplicas`                        | Supported — scalable target max capacity                 |
| `autoScaling.metrics` `cpu` / `memory`      | Supported — KEDA scale rules                           | Supported — target tracking policy                       |
| `autoScaling.metrics` `custom`              | Supported — KEDA custom scaler                         | Supported — target tracking on a CloudWatch metric       |
| `autoScaling.metrics.*.target.averageValue` | Supported — KEDA per-replica target                    | Supported — target tracking on a per-task metric         |
| `autoScaling.metrics.*.target.value`        | Supported — KEDA absolute threshold                    | **Unsupported** — target tracking needs a per-task value |
| `volumes.*.persistentVolume`                | Supported — environment storage + `template.volumes[]` | Supported — `volumes[].efsVolumeConfiguration`           |
| `volumes.*.secretName`                      | Supported — `template.volumes[]` type `Secret`         | **Unsupported** — no secrets-as-files primitive          |
| `volumes.*.emptyDir` `medium: disk`         | Supported — `storageType: EmptyDir`                    | Supported — task `volumes[]` bind mount                  |
| `volumes.*.emptyDir` `medium: memory`       | **Unsupported** — no tmpfs-backed option               | Supported — `linuxParameters.tmpfs[]`                    |
| `volumeMounts`                              | Supported — `template.containers[].volumeMounts[]`     | Supported — `mountPoints[]`                              |
| `connections`                               | Supported — `CONNECTION_*` + internal FQDN             | Supported — `CONNECTION_*` + Service Connect alias       |
| `hosts` (output)                            | Supported — environment-internal FQDN                  | Supported — Service Connect client alias                 |
| `platformOptions`                           | Supported — Container App passthrough                  | Supported — task definition passthrough                  |
| `extensions.daprSidecar`                    | Supported — native Dapr                                | **Unsupported** — ECS has no Dapr integration            |

Behavior notes:

- **`resources.requests`** — on ACA this becomes the single allocation, and the legal values depend on the environment's workload profile. On ECS the recipe resolves the task size to the nearest legal Fargate CPU and memory combination, so the value you get may be larger than the value you wrote.
- **`resources.limits`** — ACA has no separate limit; the request *is* the allocation. On ECS, container `memory` is a hard limit but there is no CPU ceiling, so a CPU limit is ignored.
- **`livenessProbe`** — no probe kind works on all three platforms. ACA has no `exec`; ECS has no container-level HTTP probe.
- **`readinessProbe`** — HTTP is portable when the workload has a route. On ECS it is enforced by the ALB, so a container with no route has no readiness gate.
- **`restartPolicy`** — ACA always restarts and is not configurable. ECS exposes a boolean, so `Always`, `OnFailure`, and `Never` cannot be distinguished.
- **`replicas` on ACA** — an exact count is expressed by setting `minReplicas` and `maxReplicas` to the same value.
- **`autoScaling` on ACA** — the floor is 1, so there is no scale to zero.
- **`autoScaling.metrics` `custom` on ECS** — the metric must already be published to CloudWatch and normalized per running task, for example queue depth divided by task count. `averageValue` becomes the target-tracking target.

#### Routes

| Property                            | Azure Container Apps                               | Amazon ECS                                           |
|-------------------------------------|----------------------------------------------------|------------------------------------------------------|
| `kind: HTTP`                        | Supported — `httpRouteConfigs.rules[]`             | Supported — ALB listener rule → target group         |
| `kind: TCP`                         | Supported — `ingress.transport: tcp`               | **Unsupported** — needs an NLB                       |
| `kind: TLS`                         | **Unsupported** — no SNI matching at the edge      | **Unsupported** — needs an NLB                       |
| `kind: UDP`                         | **Unsupported** — no UDP ingress                   | **Unsupported** — needs an NLB                       |
| `hostnames`                         | Supported — custom domain + managed certificate    | Supported — `host-header` condition                  |
| `rules.*.matches.*.httpPath`        | Supported — `match.pathSeparatedPrefix`            | Supported — `path-pattern` for `/path` and `/path/*` |
| `rules.*.matches.*.httpHeaders`     | **Unsupported** — no header matching               | Supported — `http-header` condition                  |
| `rules.*.matches.*.httpMethod`      | **Unsupported** — no method matching               | Supported — `http-request-method` condition          |
| `rules.*.matches.*.httpQueryParams` | **Unsupported** — no query matching                | Supported — `query-string` condition                 |
| `listener` (output)                 | Supported — FQDN on the environment default domain | Supported — ALB DNS name                             |

Behavior notes:

- **The L4 kinds on ECS** are unsupported because the pack consumes an ALB, which is HTTP-only. Supporting them would mean consuming an NLB as well.
- **Path matching keeps Kubernetes path-segment semantics.** A route on `/api` matches `/api` and `/api/orders` but not `/apix`, on every platform, so redeploying the same route elsewhere selects the same requests.
- **Request matching is the mirror image of each other.** ACA matches on path only; ECS matches on path, header, method, and query. An application that routes on a header is not portable to ACA.

#### Secrets

| Property                                       | Azure Container Apps                           | Amazon ECS                                     |
|------------------------------------------------|------------------------------------------------|------------------------------------------------|
| `kind: generic`                                | Supported — Key Vault secret via `keyVaultUrl` | Supported — Secrets Manager secret             |
| `kind: certificate-pem` / `certificate-pkcs12` | Supported — Key Vault secret                   | Supported — Secrets Manager secret             |
| `kind: basicAuthentication`                    | **Unsupported** — registry authentication only | **Unsupported** — registry authentication only |
| `kind: azureWorkloadIdentity`                  | **Unsupported** — ACA uses managed identity    | — not applicable                               |
| `kind: awsIRSA`                                | — not applicable                               | **Unsupported** — ECS uses task roles          |
| Rotation                                       | Supported — versionless Key Vault references   | **Unsupported** — no live reload               |

Behavior notes:

- **`certificate-*`** stores identically to `generic` on both platforms; `kind` drives data-key validation, not storage type.
- **`basicAuthentication`, `azureWorkloadIdentity`, and `awsIRSA`** are scoped to OCI registry authentication for Bicep recipe storage, not to workload identity — the schema says so explicitly.
- **Rotation** — on ACA a versionless Key Vault reference picks up a new version within \~30 min, and ACA then automatically restarts every active revision that consumes the secret through an environment variable. Rotation is therefore a restart event that users should plan for. Direct app-secret updates do not take effect until the revision is restarted or a new revision is deployed. On ECS a value change requires a new deployment.

#### Persistent volumes

| Property                            | Azure Container Apps                             | Amazon ECS                                             |
|-------------------------------------|--------------------------------------------------|--------------------------------------------------------|
| `sizeInGib`                         | Supported — Azure Files share quota              | **Unsupported** — EFS is elastic; the value is dropped |
| `allowedAccessModes: ReadWriteMany` | Supported — Azure Files share                    | Supported — EFS mount targets                          |
| `allowedAccessModes: ReadOnlyMany`  | Supported — `ReadOnly` on the storage definition | Supported — `mountPoints[].readOnly`                   |
| `allowedAccessModes: ReadWriteOnce` | **Unsupported** — Azure Files is shared          | **Unsupported** — EFS is shared                        |

Behavior notes:

- **`sizeInGib` on ECS** — the schema marks it required, so every application must set a value that ECS then ignores. This is the concrete case behind Q3.
- **`ReadOnlyMany` on ACA** — applied per storage definition, not per mount, so an application with both read-write and read-only consumers of the same share needs two definitions.
- **`ReadWriteOnce`** — neither platform can enforce single-writer exclusivity. On AWS it would require EBS, which is one volume per task and deleted when the task stops.
- **`allowedAccessModes`** is a single value, not a set — the rows enumerate the enum, and a volume requests one mode at a time.

### Feature 1: Azure Container Apps recipe pack

Publish an `azure-aca` recipe pack covering all four core types, contributed to `resource-types-contrib`. The pack consumes a pre-existing ACA managed environment and creates Container Apps, environment-level storage and route configurations, and the Key Vault it writes secrets into.

It ships with its documentation: what the pack creates, what Priya brings, the support state for every property above, and how to reach ACA-only settings through `platformOptions`.

### Feature 2: Amazon ECS recipe pack

Publish an `aws-ecs` recipe pack covering all four core types, targeting Fargate with `awsvpc` networking and `kind: HTTP` routing through an ALB. The pack consumes a pre-existing ECS cluster, VPC, subnets, security groups, execution role, ALB, EFS file system, and EFS mount targets, and creates task definitions, ECS services, target groups, listener rules, EFS access points, and Secrets Manager secrets.

It ships with its documentation: what the pack creates, what Priya brings, the support state for every property above, and how to reach ECS-only settings through `platformOptions`.

### Feature 3: Reference applications

A reference application for each platform, exercising all four core types on it — two containers with a connection between them, an HTTP route with path matching, a persistent volume, and a secret.

Each one shows what the pack actually delivers on its own platform, including the properties that behave differently there, and gives Priya something to deploy before she commits to the pack.

### Feature 4: Platform documentation

A page per container platform on [docs.radapp.io](https://docs.radapp.io/management/container-platforms/), nested under Container platforms — which today carries Azure Container Instances as a single inline example and covers neither ACA nor ECS.

Each page covers how to register the environment, what the platform engineer brings, a deployment walkthrough, where to find runtime logs and diagnostics, and the unsupported properties with their alternatives, in the same shape as the existing Azure Container Instances example.

Runtime logs and diagnostics use each platform's native tools — ACA log streaming and revision diagnostics; ECS service events and CloudWatch Logs — the same way Kubernetes users rely on `kubectl logs` and `kubectl describe` today.

This is where a developer looks first, before `resource-types-contrib`. The pack READMEs are the reference; these pages are the path in.
