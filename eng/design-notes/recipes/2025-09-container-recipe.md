# Container Resource Recipe Migration

* **Author**: Brooke Hamilton (@Brooke-Hamilton)

## Overview

This design describes replacing the imperative Go renderer chain for Applications.Core/containers with a Bicep recipe for the new Radius.Compute/containers resource type. The current implementation uses a chain of wrapper renderers (`kubernetesmetadata.Renderer` → `manualscale.Renderer` → `daprextension.Renderer` → `container.Renderer`) that generates Kubernetes Deployments, Services, Secrets, RBAC resources, and Azure identity resources. This will be replaced with a single Bicep recipe that works with the new resource schema.

## Terms and definitions

- **Container Renderer Chain**: The composed renderers in `pkg/corerp/model/application_model.go` that process containers
- **Bicep Recipe**: Declarative template that provisions infrastructure based on input parameters

## Objectives

### Goals

- Replace the container renderer chain with a Bicep recipe for the new Radius Compute/containers resource type
- Implement all current functionality for the new resource schema: multi-container deployments, volumes, identity, RBAC, connections
- Enable platform engineers to customize container deployment through recipe modifications
- Align with the new resource type schema defined in resource-types-contrib

### Non-goals

- Supporting the old Applications.Core/containers resource type
- Modifying the recipe engine or deployment processor  
- Supporting non-Kubernetes platforms in this phase
- Maintaining compatibility with existing deployed Applications.Core/containers resources
- Cascading Kubernetes metadata (labels/annotations) from Environment/Application resources to container deployments (this functionality exists today but will not be replicated in the recipe-based approach)

## User Experience

Users will define Radius.Compute/containers resources using the new schema:

```bicep
resource myContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'myContainer'
  properties: {
    environment: environment
    application: application.id
    containers: {
      web: {
        image: 'nginx:latest'
        ports: {
          http: {
            containerPort: 80
          }
        }
      }
    }
  }
}
```

The recipe produces the same Kubernetes resources as the current renderer chain but works with the new resource type schema.

## Design

### Current Architecture

The current container deployment uses a chain of renderers registered in `pkg/corerp/model/application_model.go`:

```go
&kubernetesmetadata.Renderer{
  Inner: &manualscale.Renderer{
    Inner: &daprextension.Renderer{
      Inner: &container.Renderer{
        RoleAssignmentMap: roleAssignmentMap,
      },
    },
  },
}
```

Each renderer wraps the next, adding specific functionality:
- `container.Renderer`: Core Kubernetes resources (Deployment, Service, Secret, RBAC, Identity)
- `daprextension.Renderer`: Adds Dapr sidecar annotations
- `manualscale.Renderer`: Sets replica count
- `kubernetesmetadata.Renderer`: Adds custom labels/annotations

### Target Architecture

Replace the entire renderer chain with a single Bicep recipe that leverages the **Bicep Kubernetes extension (preview)** to directly create Kubernetes resources. This approach provides:

- **Direct Kubernetes Resource Creation**: Use the Bicep Kubernetes extension to create Kubernetes resources with native API schemas
- **Type Safety**: Full Bicep IntelliSense and validation for Kubernetes resource properties  
- **Declarative Approach**: Replace imperative Go code with declarative Bicep templates
- **Development Experience**: VS Code support with automatic YAML-to-Bicep conversion using "Import Kubernetes Manifest"

#### Bicep Kubernetes Extension Integration

The recipe will utilize the **Bicep Kubernetes extension (preview)** configuration:

```bicep
@description('Kubernetes configuration from Radius environment')
param kubeConfig string

extension kubernetes with {
  kubeconfig: kubeConfig
  namespace: 'radius-system'
} as k8s
```

**Key Extension Capabilities:**
- **Resource Type Format**: `{group}/{kind}@{version}` (e.g., `apps/Deployment@v1`, `core/Service@v1`)
- **Namespace Management**: Automatic namespace handling with client-side and server-side dry run support
- **Parameter Validation**: JSON schema validation for kubeconfig and resource properties
- **Resource Discovery**: Dynamic discovery of available Kubernetes API resources and versions

### Detailed Design

#### Current Container Renderer Analysis

The `container.Renderer.Render()` method in `pkg/corerp/renderers/container/render.go` creates resources for Applications.Core/containers. The new recipe must create the same resources but for the Radius.Compute/containers schema:

1. **Kubernetes Deployment** (`rpv1.LocalIDDeployment`)
   - Multi-container support via `properties.containers` map (matches new schema)
   - Health probes (readiness/liveness) via `makeHealthProbe()`
   - Environment variables from connections and direct values
   - Volume mounts for ephemeral and persistent volumes
   - Base manifest merging via `kubeutil.ParseManifest()`
   - Pod spec patching from `properties.runtimes.kubernetes.pod`

2. **Kubernetes Service** (`rpv1.LocalIDService`) 
   - Created only when containers expose ports
   - ClusterIP service with port mapping

3. **Kubernetes Secret** (`rpv1.LocalIDSecret`)
   - Connection environment variables
   - Secret data from `getEnvVarsAndSecretData()`

4. **Identity Resources** (when `identityRequired = true`)
   - Azure Managed Identity (`rpv1.LocalIDUserAssignedManagedIdentity`)
   - Federated Identity Credential (`rpv1.LocalIDFederatedIdentity`) 
   - Service Account (`rpv1.LocalIDServiceAccount`)

5. **RBAC Resources**
   - Kubernetes Role (`rpv1.LocalIDKubernetesRole`) - secrets get/list permissions
   - RoleBinding (`rpv1.LocalIDKubernetesRoleBinding`)

6. **Volume Resources**
   - SecretProviderClass for Azure Key Vault volumes
   - Role assignments for Key Vault access

#### Recipe Parameter Structure

Based on the new Radius.Compute/containers schema:

```bicep
param context object              // Resource metadata, environment settings  
param containers object           // Multi-container specification (required)
param connections object = {}     // Resource connections (optional)
param volumes object = {}         // Volume configurations (optional)
param restartPolicy string = 'Always'  // Container restart policy
param replicas int = 1            // Number of replicas (optional)
param autoScaling object = {}     // Auto-scaling configuration (optional)
param extensions object = {}      // Extensions like daprSidecar (optional)
param platformOptions object = {} // Platform-specific properties (optional)
```

**Volume Handling Responsibilities:**

The recipe handles three types of volumes with different responsibilities, following the Kubernetes volume conventions:

1. **Ephemeral Volumes (emptyDir)**: Volumes specified with the `emptyDir` property are created directly by the recipe in the Deployment spec. These are temporary volumes that exist only for the lifetime of the pod. The `emptyDir` property follows the Kubernetes emptyDir convention.

2. **Persistent Volumes**: Volumes specified with the `persistentVolume` property reference PersistentVolumeClaims (PVCs) that are created by the `Radius.Compute/persistentVolumes` resource type. The recipe receives the volume resource ID through `persistentVolume.resourceId` and mounts the corresponding PVC using `persistentVolumeClaim` volume sources.

3. **Secret Volumes**: Volumes specified with the `secretId` property reference `Radius.Security/secrets` resources. The recipe mounts these as Kubernetes secrets for sensitive data that should not be exposed as environment variables.

#### Critical Implementation Challenges

1. **Multi-Container Complexity**: The new containers schema will support multiple containers. The recipe must iterate over the `containers` map to create container specs in the Deployment. (If other platforms only support single containers, then the recipe for that platform must throw an error upon deployment.)

2. **Extension Renderer Chain**: The current architecture uses wrapper renderers for extensions. The recipe must replicate:
   - Dapr sidecar annotations (`dapr.io/enabled`, `dapr.io/app-id`, etc.)
   - Manual scaling replica settings
   - Kubernetes metadata (custom labels/annotations) - Note: In the new schema, Kubernetes metadata moves from extensions to `platformOptions.kubernetes.metadata`

3. **Environment Variable Processing**: Complex logic in `getEnvVarsAndSecretData()`:
   - Connection-based environment variables with naming patterns
   - URL parsing for connection sources  
   - Type conversion and JSON marshaling for complex values
   - Secret reference handling

4. **Volume Handling**: The recipe has different responsibilities for volume types following the Kubernetes volume conventions:
   - **Ephemeral volumes (emptyDir)**: Recipe creates volume definitions directly using the `emptyDir` property, following Kubernetes emptyDir convention
   - **Persistent volumes**: Recipe mounts pre-existing PersistentVolumeClaims (PVCs) created by the Radius.Compute/persistentVolumes resource type, referenced via `persistentVolume.resourceId`
   - **Secret volumes**: Recipe mounts Kubernetes secrets as volumes, referenced via `secretId` pointing to Radius.Security/secrets resources
   - **Dynamic RP Support Required**: The volumes construct in the new schema (where containers reference volume resources via `persistentVolume.resourceId` or `secretId`) requires the dynamic resource provider functionality to resolve volume resource references and pass the necessary mounting information (e.g., PVC names, mount paths) to the container recipe
   - Volume resource property extraction for mount configuration

5. **Identity and RBAC Logic**:
   - Conditional identity creation based on connections/volumes
   - Azure resource naming conventions (`azrenderer.MakeResourceName()`)
   - Role assignment scope determination
   - Service account workload identity configuration

6. **Base Manifest Merging**: Current implementation uses:
   - `kubeutil.ParseManifest()` to deserialize YAML
   - Object merging for Deployment, Service, ServiceAccount bases
   - Strategic merge patch for PodSpec (`patchPodSpec()`)

#### Recipe Resource Generation Using Kubernetes Extension

The recipe leverages the Bicep Kubernetes extension to generate resources with native Kubernetes API schemas:

**Extension Configuration:**
```bicep
@description('Container configuration from Radius resource')
param containerSpec object
@description('Kubernetes configuration')
param kubeConfig string
@description('Target namespace')
param targetNamespace string = 'default'

// Configure Kubernetes extension
extension kubernetes with {
  kubeconfig: kubeConfig
  namespace: targetNamespace
} as k8s

// Variables for extension functionality replication
var commonLabels = {
  'app.kubernetes.io/name': containerSpec.name
  'app.kubernetes.io/managed-by': 'radius'
  'radapp.io/application': containerSpec.application ?? containerSpec.name
}

// Dapr extension support
var daprEnabled = containerSpec.extensions?.dapr?.enabled ?? false
var daprLabels = daprEnabled ? {
  'dapr.io/enabled': 'true'
  'dapr.io/app-id': containerSpec.extensions.dapr.appId ?? containerSpec.name
} : {}

var daprAnnotations = daprEnabled ? {
  'dapr.io/enabled': 'true'
  'dapr.io/app-id': containerSpec.extensions.dapr.appId ?? containerSpec.name
  'dapr.io/app-port': string(containerSpec.extensions.dapr.appPort ?? 80)
  'dapr.io/config': containerSpec.extensions.dapr.config ?? 'default'
} : {}

// Final labels combining base, platformOptions metadata, and Dapr
// Note: Kubernetes metadata moved from extensions to platformOptions in new schema
var finalLabels = union(
  commonLabels,
  containerSpec.platformOptions?.kubernetes?.metadata?.labels ?? {},
  daprLabels
)

var finalAnnotations = union(
  containerSpec.platformOptions?.kubernetes?.metadata?.annotations ?? {},
  daprAnnotations
)
```

**ServiceAccount with Azure Workload Identity:**
```bicep
resource serviceAccount 'core/ServiceAccount@v1' = {
  metadata: {
    name: containerSpec.name
    namespace: targetNamespace
    labels: finalLabels
    annotations: union(finalAnnotations, containerSpec.identity?.azure?.enabled ?? false ? {
      'azure.workload.identity/client-id': containerSpec.identity.azure.clientId
    } : {})
  }
}
```

**RBAC Resources:**
```bicep
resource role 'rbac.authorization.k8s.io/Role@v1' = {
  metadata: {
    name: containerSpec.name
    namespace: targetNamespace
    labels: finalLabels
  }
  rules: [
    {
      apiGroups: ['']
      resources: ['pods', 'services', 'configmaps', 'secrets']
      verbs: ['get', 'list', 'watch']
    }
  ]
}

resource roleBinding 'rbac.authorization.k8s.io/RoleBinding@v1' = {
  metadata: {
    name: containerSpec.name
    namespace: targetNamespace
    labels: finalLabels
  }
  subjects: [
    {
      kind: 'ServiceAccount'
      name: serviceAccount.metadata.name
      namespace: targetNamespace
    }
  ]
  roleRef: {
    kind: 'Role'
    name: role.metadata.name
    apiGroup: 'rbac.authorization.k8s.io'
  }
}
```

**Multi-Container Deployment with Extensions:**
```bicep
resource deployment 'apps/Deployment@v1' = {
  metadata: {
    name: containerSpec.name
    namespace: targetNamespace
    labels: finalLabels
    annotations: finalAnnotations
  }
  spec: {
    // Manual scaling extension support
    replicas: containerSpec.extensions?.manualScale?.replicas ?? 1
    selector: {
      matchLabels: commonLabels
    }
    template: {
      metadata: {
        labels: finalLabels
        annotations: finalAnnotations
      }
      spec: {
        serviceAccountName: serviceAccount.metadata.name
        containers: [for (container, i) in containerSpec.containers: {
          name: container.name ?? '${containerSpec.name}-${i}'
          image: container.image
          ports: [for port in (container.ports ?? []): {
            containerPort: port.containerPort
            name: port.name ?? 'port-${port.containerPort}'
            protocol: port.protocol ?? 'TCP'
          }]
          env: buildEnvironmentVariables(container, containerSpec.connections)
          resources: {
            limits: container.resources?.limits ?? {
              cpu: '500m'
              memory: '512Mi'
            }
            requests: container.resources?.requests ?? {
              cpu: '100m'
              memory: '128Mi'
            }
          }
          readinessProbe: container.readinessProbe
          livenessProbe: container.livenessProbe
          volumeMounts: [for mount in (container.volumeMounts ?? []): mount]
        }]
        volumes: buildVolumes(containerSpec.volumes)
      }
    }
  }
}
```

**Service for Network Access:**
```bicep
resource service 'core/Service@v1' = if (length(containerSpec.ports ?? []) > 0) {
  metadata: {
    name: containerSpec.name
    namespace: targetNamespace
    labels: finalLabels
  }
  spec: {
    selector: commonLabels
    ports: [for port in containerSpec.ports: {
      name: port.name ?? 'port-${port.port}'
      port: port.port
      targetPort: port.targetPort ?? port.port
      protocol: port.protocol ?? 'TCP'
    }]
    type: containerSpec.networking?.expose ?? false ? 'LoadBalancer' : 'ClusterIP'
  }
}
```

#### Resource Creation Logic

The recipe implements the same logic as the current Go renderer:

**Deployment Creation:**
```bicep
// Generate deployment with identical metadata and specification
resource deployment 'apps/Deployment@v1' = {
  metadata: {
    name: context.resource.name
    namespace: environment.namespace
    labels: {
      'app.kubernetes.io/name': context.resource.name
      'app.kubernetes.io/part-of': application.name
      'app.kubernetes.io/managed-by': 'radius'
    }
  }
  spec: {
    selector: {
      matchLabels: {
        app: application.name
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: application.name
          resource: context.resource.name
          'azure.workload.identity/use': identityRequired ? 'true' : null
        }
      }
      spec: {
        serviceAccountName: identityRequired ? context.resource.name : null
        enableServiceLinks: false
        containers: [
          {
            name: context.resource.name
            image: container.image
            ports: [for port in items(container.ports): {
              containerPort: port.value.containerPort
              protocol: 'TCP'
            }]
            env: concat(
              // Direct environment variables
              [for env in items(container.env): {
                name: env.key
                value: env.value.value
              } if env.value.value != null],
              // Secret-referenced environment variables  
              [for env in items(container.env): {
                name: env.key
                valueFrom: {
                  secretKeyRef: {
                    name: context.resource.name
                    key: env.key
                  }
                }
              } if env.value.valueFrom != null]
            )
            volumeMounts: [for volume in items(volumes): {
              name: volume.key
              mountPath: volume.value.mountPath
            }]
            readinessProbe: container.readinessProbe
            livenessProbe: container.livenessProbe
          }
        ]
        volumes: concat(
          // Ephemeral volumes - recipe creates these directly using emptyDir property
          [for volume in items(volumes): {
            name: volume.key
            emptyDir: volume.value.emptyDir ?? {}
          } if volume.value.?emptyDir != null],
          // Persistent volumes - recipe mounts PVCs created by persistentVolume resources
          [for volume in items(volumes): {
            name: volume.key
            persistentVolumeClaim: {
              claimName: volume.value.persistentVolume.resourceId // Reference to PVC created by Radius.Compute/persistentVolumes
            }
          } if volume.value.?persistentVolume != null],
          // Secret volumes - recipe mounts Kubernetes secrets
          [for volume in items(volumes): {
            name: volume.key
            secret: {
              secretName: volume.value.secretId // Reference to Radius.Security/secrets
            }
          } if volume.value.?secretId != null]
        )
      }
    }
  }
}
```

#### Extension Handling Complexity

Each extension requires specific processing that must be replicated in the recipe:

**Dapr Sidecar Extension** (`daprextension.Renderer`):
```bicep
// Annotations added to pod template
var daprAnnotations = contains(extensions, 'daprSidecar') ? {
  'dapr.io/enabled': 'true'
  'dapr.io/app-id': extensions.daprSidecar.?appId ?? context.resource.name
  'dapr.io/app-port': string(extensions.daprSidecar.?appPort ?? '')
  'dapr.io/config': extensions.daprSidecar.?config ?? ''
  'dapr.io/protocol': extensions.daprSidecar.?protocol ?? 'http'
} : {}
```

**Manual Scaling Extension** (`manualscale.Renderer`):
```bicep
// Replica count setting
var replicaCount = contains(extensions, 'manualScaling') 
  ? extensions.manualScaling.replicas 
  : (replicas ?? 1)
```

**Kubernetes Metadata** (moved from extensions to `platformOptions`):
```bicep
// Custom labels and annotations - now in platformOptions.kubernetes.metadata
var customLabels = platformOptions.?kubernetes.?metadata.?labels ?? {}
var customAnnotations = platformOptions.?kubernetes.?metadata.?annotations ?? {}
```

**Notes**: 
- In the new Radius.Compute/containers schema, Kubernetes metadata (labels and annotations) has been moved from the `extensions` array to `platformOptions.kubernetes.metadata` to better align with the platform-specific configuration model.
- **Breaking Change**: The current implementation supports cascading Kubernetes metadata from Environment and Application resources to container deployments. This cascading behavior will NOT be replicated in the recipe-based approach. Platform engineers who need environment-level or application-level metadata must configure it through recipe parameters in the Environment definition, and the recipe must explicitly apply those labels/annotations to all generated resources.

#### Resource Provisioning

The new Radius.Compute/containers schema uses recipes by default, eliminating the need for manual vs internal provisioning modes. All resources are created through the recipe.

#### Base Manifest Integration

Complex merging logic from `manifest.go` must be replicated:

```bicep
// Parse base manifest (no direct Bicep equivalent to kubeutil.ParseManifest)
var baseManifest = runtimes.?kubernetes.?base ?? ''
var deploymentBase = parseJson(baseManifest) // Limited Bicep YAML parsing

// Merge with generated deployment
var finalDeployment = union(deploymentBase, generatedDeployment)

// Pod spec patching (no Bicep equivalent to strategicpatch.StrategicMergePatch)
var podPatch = runtimes.?kubernetes.?pod ?? ''
// This requires custom JSON merging logic in Bicep
```

#### Implementation Complexity Analysis

**Bicep Kubernetes Extension Advantages:**

1. **Native Kubernetes Resources**: Direct creation using official Kubernetes API schemas with full type safety
2. **VS Code Integration**: "Import Kubernetes Manifest" command enables easy YAML-to-Bicep conversion 
3. **Type Safety & IntelliSense**: Full Bicep validation and autocompletion for Kubernetes resource properties
4. **Namespace Management**: Automatic namespace handling with client-side and server-side dry run support
5. **Resource Discovery**: Dynamic discovery of available Kubernetes API resources and versions

**Manageable Complexity Areas:**

1. **Extension Functionality Replication**: 
   - Dapr: Use conditional annotations and labels in Bicep
   - Manual Scaling: Simple replica count parameter
   - Kubernetes Metadata: Direct label/annotation merging

2. **Multi-Container Support**: Native support through Bicep array iteration over containers

3. **Resource Dependencies**: Handled through Bicep resource references and dependency ordering

4. **Identity Integration**: Azure workload identity annotations on ServiceAccount resources

**Reduced Complexity with Extension:**

1. **Direct Resource Creation**: No need for complex YAML parsing - use native Kubernetes resource definitions
2. **Type Validation**: Bicep + Kubernetes extension provides compile-time validation 
3. **Declarative Approach**: Replace complex imperative Go logic with declarative Bicep templates
4. **Resource Relationships**: Natural Bicep dependency management vs manual renderer coordination

**Remaining Challenges:**

1. **Environment Variable Processing**: Complex type conversion logic still needs Bicep implementation
2. **Base Manifest Integration**: May need alternative approach for user-provided YAML merging
3. **Extension Preview Status**: Kubernetes extension is in preview and may have limitations

#### Risk Assessment

**Medium Risk**: The Bicep Kubernetes extension significantly reduces implementation complexity:
- **Reduced**: Direct Kubernetes resource creation vs complex Go renderer chains
- **Simplified**: Type-safe declarative templates vs imperative resource generation
- **Improved**: VS Code tooling support with IntelliSense and validation
- **Concern**: Extension preview status may introduce stability risks
- Limited Bicep capabilities for complex logic

### Resource Registration Changes

Current registration in `pkg/corerp/model/application_model.go` for Applications.Core/containers:

```go
{
  ResourceType: container.ResourceType, // "Applications.Core/containers"
  Renderer: &mux.Renderer{...}
}
```

New registration will target Radius.Compute/containers with recipe configuration:

```go
{
  ResourceType: "Radius.Compute/containers",
  Recipe: RecipeConfiguration{
    TemplateName: "containers",
    Parameters: containerParameterMapping,
  },
}
```

### Implementation Plan

#### Phase 1: Recipe Development

**Core Recipe Creation:**
1. Create `containers.bicep` recipe with multi-container support
2. Implement resource generation logic:
   - Kubernetes Deployment with container array
   - Conditional Service creation
   - Secret generation for connections
   - RBAC Role/RoleBinding creation

**Extension Integration:**
3. Replicate extension functionality in recipe:
   - Dapr sidecar annotations
   - Manual scaling replica count
   - Kubernetes metadata labels/annotations

**Identity and Volume Support:**
4. Implement conditional identity creation
5. Add volume mounting logic
6. Handle persistent volume dependencies

#### Phase 2: Advanced Features

**Base Manifest Support:**
1. Develop Bicep-based manifest merging (limited capabilities)
2. Implement pod spec patching approximation
3. Handle deployment/service/serviceaccount base objects

**Environment Variable Processing:**
4. Replicate connection-based environment variables
5. Implement type conversion logic for complex values
6. Add secret reference handling

#### Phase 3: Migration

**Registration Updates:**
1. Add recipe registration for Radius.Compute/containers  
2. Create parameter mapping from new schema to recipe parameters
3. Remove Applications.Core/containers renderer registration

**Cleanup:**
4. Applications.Core/containers renderer chain can remain for existing deployments
5. Update tests for new resource type
6. Update documentation for Radius.Compute/containers

### Breaking Changes

#### Kubernetes Metadata Cascading

**Current Behavior**: The existing Applications.Core/containers implementation supports cascading Kubernetes metadata (labels and annotations) from Environment and Application resources to container deployments. This means labels/annotations defined at the environment or application level are automatically applied to all container resources within that scope.

**New Behavior**: The recipe-based approach for Radius.Compute/containers will NOT support automatic cascading of Kubernetes metadata. This is a breaking change that requires platform engineers to explicitly handle metadata propagation.

**Migration Path**: Platform engineers who need environment-level or application-level metadata must:
1. Define metadata in the Environment's recipe parameters (e.g., `recipeParameters.Radius.Compute/containers.metadata.labels`)
2. Update recipes to merge environment-level metadata with container-specific metadata from `platformOptions.kubernetes.metadata`
3. Ensure the recipe applies the merged metadata to all generated Kubernetes resources (Deployment, Service, etc.)

**Example Environment Configuration**:
```bicep
resource env 'Radius.Core/environments@2025-05-01-preview' = {
  name: 'my-env'
  properties: {
    recipePacks: [kubernetesRecipePack.id]
    recipeParameters: {
      'Radius.Compute/containers': {
        allowPlatformOptions: true
        // Environment-level metadata that recipes should apply to all containers
        metadata: {
          labels: {
            'team.contact.name': 'platform-team'
            'cost-center': '12345'
          }
        }
      }
    }
  }
}
```

**Impact**: Users who rely on metadata cascading will need to update their Environment configurations and potentially customize recipes to achieve the same behavior.

### Technical Challenges and Gaps

#### Critical Missing Capabilities

1. **Schema Translation**: Complex mapping from Applications.Core/containers internal model to Radius.Compute/containers schema
2. **Extension Structure**: New schema uses different extension structure requiring significant recipe logic changes
3. **Complex Type Processing**: Limited ability to replicate Go's flexible type conversion in environment variable processing
4. **Volume Resolution**: The new schema's volumes construct (where containers reference separate volume resources via `properties.volumes`) requires dynamic resource provider functionality to resolve these references at deployment time. The container recipe needs access to volume resource outputs (e.g., PVC names from Radius.Compute/persistentVolumes resources) to properly mount them. This cross-resource dependency resolution is not available in the current recipe system.
5. **Metadata Cascading**: Current implementation cascades Kubernetes metadata (labels/annotations) from Environment and Application resources to container deployments. This functionality will NOT be replicated in recipes. Platform engineers must explicitly configure environment-level metadata through recipe parameters.

#### Workarounds Required

1. **Recipe Engine Enhancement**: Pre-process dependencies and schema mapping before recipe execution
2. **Parameter Expansion**: Flatten complex objects into recipe-compatible parameters
3. **Extension Mapping**: Convert between extension array model (current) and object model (new schema)
4. **Simplified Type Support**: Support only basic types in environment variables initially
5. **Monolithic Recipe**: Single recipe handling all functionality vs modular renderer composition
6. **Metadata Configuration Pattern**: Platform engineers must explicitly configure environment-level or application-level Kubernetes metadata through recipe parameters instead of relying on automatic cascading. The recipe must be designed to merge environment-level metadata with container-specific metadata.
7. **Dynamic RP for Volume References**: The recipe engine must support dynamic resource provider functionality to resolve cross-resource dependencies. Specifically, when a container references volumes via `properties.volumes`, the recipe needs access to outputs from the referenced Radius.Compute/persistentVolumes resources (e.g., PVC names) to generate proper volume mounts.

#### Development Effort Estimate

- **High Complexity**: 4-6 months for experienced developer
- **Risk Factors**: Schema mapping complexity, Bicep limitations, recipe engine enhancements
- **Testing Overhead**: Comprehensive testing with new resource type schema

## Test Plan

### Functional Implementation Tests

1. **Multi-Container Deployment**: Test containers map with multiple containers, shared volumes, different configurations using new schema
2. **Extension Functionality**: Verify Dapr sidecars, scaling, auto-scaling work with new extension structure
3. **Volume Integration**: 
   - Test ephemeral volumes (emptyDir) created by recipe
   - Test persistent volumes mounted from PVCs created by Radius.Compute/persistentVolumes resources
   - Test secret volumes mounted from Kubernetes secrets
   - Verify volume mount paths and configurations
4. **Identity and RBAC**: Verify Azure workload identity, service accounts, role bindings work with recipe
5. **Connection Processing**: Test environment variable generation from connections using new schema
6. **Platform Options**: Test platform-specific properties for Kubernetes

### New Schema Validation

1. **Schema Compliance**: Verify recipe works with all Radius.Compute/containers schema fields
2. **Auto-scaling**: Test new auto-scaling configuration (maxReplicas, metrics, targets)
3. **Resource Constraints**: Test CPU/memory requests and limits
4. **Extension Structure**: Test new extension object structure vs old array structure
5. **Error Scenarios**: Invalid configurations, missing required fields

### Recipe-Specific Testing

1. **Parameter Mapping**: Verify correct translation from resource properties to recipe parameters
2. **Resource Output**: Validate recipe produces expected Kubernetes resources
3. **Performance Impact**: Measure recipe execution time and resource usage


## Recommended Implementation Strategy

1. **Enable Extension Support**: Configure bicepconfig.json with experimental extensibility features
2. **Iterative Development**: Start with core deployment, add extension functionality progressively
3. **VS Code Integration**: Leverage "Import Kubernetes Manifest" for existing YAML conversion
4. **Comprehensive Testing**: Validate extension preview stability and performance characteristics
5. **Documentation**: Create migration guides and best practices for Kubernetes extension usage

## Risk Mitigation

1. **Extension Preview Status**: Monitor extension stability and provide fallback options
2. **Performance Validation**: Benchmark recipe execution vs current Go renderer performance  
3. **Feature Parity**: Ensure all current functionality is preserved in Kubernetes extension implementation

## Open Questions

1. **Extension Capability Gaps**: Are there any current renderer capabilities that cannot be replicated with the Kubernetes extension? We believe that all critical capabilities can be replicated, but we will be looking for confirmation during implementation.
2. **Dynamic RP for Volume Dependencies**: How will the recipe engine support dynamic resource provider functionality to resolve cross-resource dependencies? Specifically, how will container recipes access outputs from referenced Radius.Compute/persistentVolumes resources to obtain PVC names and other mounting information?

## Appendix: Technical Details

### Current Renderer Chain Analysis

**File**: `pkg/corerp/model/application_model.go:99-109`
```go
&kubernetesmetadata.Renderer{
  Inner: &manualscale.Renderer{
    Inner: &daprextension.Renderer{
      Inner: &container.Renderer{
        RoleAssignmentMap: roleAssignmentMap,
      },
    },
  },
}
```

**Extension Processing Order**: metadata → scaling → dapr → container (inner-to-outer execution)

**Note**: In the new schema, Kubernetes metadata is no longer in the extension chain but is specified via `platformOptions.kubernetes.metadata`

### Resource Type Schema Differences

**Current**: Applications.Core/containers (internal Go model)
**Target**: Radius.Compute/containers (new public schema)

Key schema differences requiring recipe parameter mapping:
- Multi-container model: `properties.containers` (object map)
- Extension structure: `properties.extensions.daprSidecar` (object) vs old array model
- Volume configuration: `properties.volumes` with three distinct properties following Kubernetes conventions:
  - `emptyDir`: Ephemeral volumes (follows Kubernetes emptyDir convention)
  - `persistentVolume.resourceId`: Reference to Radius.Compute/persistentVolumes resource
  - `secretId`: Reference to Radius.Security/secrets resource
- Scaling options: `properties.replicas` and `properties.autoScaling` (direct properties)
- Resource constraints: `properties.containers[name].resources.requests/limits`
- Platform options: `properties.platformOptions` for platform-specific configuration
  - Kubernetes metadata moved from `extensions.kubernetesMetadata` to `platformOptions.kubernetes.metadata`
  - Platform-specific pod configurations in `platformOptions.kubernetes` (e.g., tolerations, nodeSelector)

### Computed Values System

The current renderer uses computed values with transformers:
```go
computedValues[handlers.IdentityProperties] = rpv1.ComputedValueReference{
  Value: options.Environment.Identity,
  Transformer: func(r v1.DataModelInterface, cv map[string]any) error {
    // Update resource with computed identity info
  },
}
```

Recipes cannot replicate this post-processing capability.