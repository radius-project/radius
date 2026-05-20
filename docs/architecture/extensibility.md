# Radius Extensibility Architecture

Radius is extended by registering **resource types** (the API surface) and
**recipes** (the implementation that provisions infrastructure for those
types). Once both are in place, a Bicep deployment that references a resource
of that type is routed by UCP to the dynamic resource provider, which selects
the appropriate recipe from the target environment and runs it through a
driver.

This document covers the three flows that make extensibility work:

1. How resource types are registered.
2. How recipes are registered.
3. How types and recipes are invoked during deployment.

## High-Level View

```mermaid
graph TD
  Author[Resource type author]
  RecipeAuthor[Recipe author / operator]
  CLI["rad CLI<br/>pkg/cli/cmd/resourcetype<br/>pkg/cli/cmd/recipe"]
  Manifest["Manifest YAML<br/>Radius.&lt;Category&gt;/&lt;type&gt;.yaml"]
  Initializer["UCP initializer<br/>pkg/ucp/initializer"]
  UCP["UCP frontend<br/>pkg/ucp/frontend"]
  DB["Database<br/>System.Resources/*"]
  Env["Applications.Core/environments<br/>(stores recipe entries)"]
  Proxy["UCP ProxyController<br/>radius/proxy.go"]
  Location["Location metadata<br/>downstream address + API versions"]
  DynRP["dynamic-rp<br/>pkg/dynamicrp"]
  DynFrontend["dynamic-rp frontend<br/>DefaultAsyncPut/Delete"]
  Queue["Status manager + async queue"]
  Worker["dynamic-rp backend worker"]
  DynCtrl["DynamicResourceController<br/>dynamicrp/backend/controller"]
  Engine["Recipe engine<br/>pkg/recipes/engine"]
  Drivers["Drivers<br/>bicep / terraform"]
  DeploymentEngine["Deployment engine<br/>(Bicep driver)"]
  Infra[Target infrastructure]

  Author --> Manifest
  Manifest --> CLI
  Manifest --> Initializer
  CLI --> UCP
  UCP --> DB
  Initializer --> DB
  RecipeAuthor --> CLI
  CLI --> Env
  Env --> DB

  UCP --> Proxy
  Proxy --> Location
  Location --> DB
  Proxy --> DynRP
  DynRP --> DynFrontend
  DynFrontend --> Queue
  Queue --> Worker
  Worker --> DynCtrl
  DynCtrl --> Engine
  Engine --> Env
  Engine --> Drivers
  Drivers --> DeploymentEngine
  Drivers --> Infra
```

### Key Components

- **Resource type manifest** — YAML describing the namespace
  (`Radius.<Category>`), one or more types, API versions, OpenAPI schemas, and
  capabilities. Source of truth for the type.
- **UCP `System.Resources` provider** — internal provider that owns the
  `resourceProviders`, `resourceTypes`, `apiVersions`, and `locations`
  resources used for routing and validation.
- **dynamic-rp** — the generic resource provider that handles any registered
  resource type without a dedicated implementation
  ([dynamic-rp.md](dynamic-rp.md)).
- **Environment** — `Applications.Core/environments` resource that holds the
  per-type recipe map (`properties.recipes[<type>][<recipeName>]`).
- **Recipe engine** — selects a driver and runs the recipe to produce
  resources, values, and secrets.

## Resource Type Registration

A resource type is registered by writing its manifest into UCP under the
`System.Resources` provider. There are two paths to do this; both end at the
same database records.

### Path A: `rad resource-type create`

Used by authors registering types into an existing cluster.

- Command: [pkg/cli/cmd/resourcetype/create/create.go](../../pkg/cli/cmd/resourcetype/create/create.go)
- Manifest parse / validate: [pkg/cli/manifest/validation.go](../../pkg/cli/manifest/validation.go), [pkg/cli/manifest/parser.go](../../pkg/cli/manifest/parser.go)
- Provider + types + API versions + location calls: [pkg/cli/manifest/registermanifest.go](../../pkg/cli/manifest/registermanifest.go)
- UCP client: `pkg/ucp/api/v20231001preview`

`rad resource-type create` calls UCP through the generated client factory in
this order:

1. `EnsureResourceProviderExists` — fetch the namespace (e.g.
  `Radius.Compute`), or create the resource provider and an empty location if
  it does not exist.
2. `RegisterType` — register each selected type from the manifest.
3. `ResourceTypesClient.BeginCreateOrUpdate` — create the type (e.g.
   `containers`) with its capabilities, default API version, and description.
4. `APIVersionsClient.BeginCreateOrUpdate` — create one entry per API version
   with the OpenAPI schema attached.
5. `LocationsClient.BeginCreateOrUpdate` — update the location resource so the
   type appears under that location, optionally with a downstream `address`
   that points UCP at a specific RP. If no address is set, UCP routes to its
   default downstream (dynamic-rp).

### Path B: UCP initializer

Used during control-plane startup to seed built-in and bundled types.

- Service: [pkg/ucp/initializer/service.go](../../pkg/ucp/initializer/service.go)
- Built-in core schemas: [pkg/ucp/initializer/radius_core_openapi.go](../../pkg/ucp/initializer/radius_core_openapi.go)

The initializer scans a manifest directory, merges files by namespace (so
`containers.yaml` and `persistentVolumes.yaml` collapse into one
`Radius.Compute` provider), and writes `ResourceProvider`, `ResourceType`,
`APIVersion`, `Location`, and `ResourceProviderSummary` records **directly to
the database**, bypassing the HTTP API and async queue. This path exists so
the cluster boots into a known good state without going through itself.

### Storage Layout

All records live under the local Radius plane:

```text
/planes/radius/local/providers/System.Resources/resourceProviders/<Namespace>
  /resourceTypes/<typeName>
    /apiVersions/<version>
  /locations/<locationName>
```

The `Location` resource is what UCP consults at request time to decide where
to proxy a request for that type.

### Registration Flow

```mermaid
sequenceDiagram
  participant Dev as Author
  participant CLI as rad CLI
  participant UCP as UCP frontend
  participant DB as Database (System.Resources)
  participant Init as UCP initializer

  Note over Dev,Init: Path A — interactive
  Dev->>CLI: rad resource-type create --from-file foo.yaml
  CLI->>CLI: ValidateManifest
  CLI->>UCP: ResourceProviders.Get
  opt Provider missing
    CLI->>UCP: ResourceProviders.CreateOrUpdate
    CLI->>UCP: Locations.CreateOrUpdate (empty location)
  end
  CLI->>UCP: ResourceTypes.CreateOrUpdate
  CLI->>UCP: APIVersions.CreateOrUpdate (per version)
  CLI->>UCP: Locations.CreateOrUpdate (attach type)
  UCP->>DB: persist records

  Note over Dev,Init: Path B — startup seed
  Init->>Init: read manifest directory, merge by namespace
  Init->>DB: write ResourceProvider / Type / APIVersion / Location directly
```

## Recipe Registration

Recipes are not stored as their own UCP resource. They are entries on an
**Environment** resource, keyed by the resource type the recipe targets.

- Command: [pkg/cli/cmd/recipe/register/register.go](../../pkg/cli/cmd/recipe/register/register.go)
- Environment client: `pkg/cli/clients` (`CreateOrUpdateEnvironment`)
- Environment data model / properties: `pkg/corerp/datamodel`, `pkg/corerp/api/v20231001preview`

`rad recipe register` does the following:

1. Fetches the target environment.
2. Builds either a `BicepRecipeProperties` or `TerraformRecipeProperties`
   value depending on the `--template-kind` flag, populating
   `TemplateKind`, `TemplatePath`, optional `TemplateVersion` /
   `PlainHTTP`, and any `Parameters`.
3. Inserts the recipe under
   `envResource.Properties.Recipes[<resourceType>][<recipeName>]`.
4. Calls `CreateOrUpdateEnvironment` to persist.

The resulting environment shape:

```yaml
properties:
  recipes:
    Radius.Data/redisCaches:
      default:
        templateKind: bicep
        templatePath: ghcr.io/.../redis:1.0.0
        parameters: { ... }
      memorydb:
        templateKind: terraform
        templatePath: git::https://...
        templateVersion: v1.2.0
```

The same map can be edited directly (e.g. by Bicep that defines the
environment) — the CLI is just a convenience that performs the merge and
update.

Recipe names are selected by the resource being deployed. For dynamic
resources, omitting `properties.recipe` does not disable recipes;
[DynamicResource.GetRecipe](../../pkg/dynamicrp/datamodel/dynamicresource.go)
returns a recipe named `default` when the property is absent. That means an
environment recipe named `default` is the implicit convention for resource
types that should work without an explicit recipe block.

## Deployment Invocation

Once the type and at least one recipe exist, a user deploys a Bicep file that
references the type, e.g.:

```bicep
resource cache 'Radius.Data/redisCaches@2025-08-01-preview' = {
  name: 'mycache'
  properties: {
    environment: env.id
    application: app.id
    recipe: { name: 'default' }
  }
}
```

The full invocation chain is:

```mermaid
sequenceDiagram
  participant User
  participant DE as Deployment engine
  participant UCP as UCP ProxyController
  participant DB as Database
  participant DynFE as dynamic-rp frontend
  participant Async as Status manager / Queue
  participant Worker as dynamic-rp backend worker
  participant DynCtrl as DynamicResourceController
  participant RecipeCtrl as RecipePutController / portableresources CreateOrUpdate
  participant CfgLoader as configloader
  participant Eng as Recipe engine
  participant Drv as bicep or terraform driver
  participant Infra as Target infra

  User->>DE: rad deploy app.bicep
  DE->>UCP: PUT /planes/radius/local/.../Radius.Data/redisCaches/mycache?api-version=...
  UCP->>DB: lookup Location for Radius.Data/redisCaches
  DB-->>UCP: Location (address = dynamic-rp URL or empty)
  UCP->>DynFE: proxy PUT (default downstream when no address)
  DynFE->>DB: save accepted resource
  DynFE->>Async: queue async PUT operation
  DynFE-->>UCP: 201/202 ARM async response
  UCP-->>DE: proxied ARM async response
  Async->>Worker: dequeue operation
  Worker->>DynCtrl: dispatch to default controller
  DynCtrl->>DynCtrl: validate body against APIVersion schema
  DynCtrl->>DynCtrl: select controller from capabilities
  alt ManualResourceProvisioning
    DynCtrl->>DynCtrl: NewInertPutController (no recipe run)
  else default (recipe-backed)
    DynCtrl->>RecipeCtrl: NewRecipePutController
    RecipeCtrl->>CfgLoader: LoadConfiguration(environmentID, applicationID, resourceID)
    RecipeCtrl->>Eng: Execute(ResourceMetadata, PrevState)
    Eng->>CfgLoader: LoadConfiguration(ResourceMetadata)
    Eng->>CfgLoader: LoadRecipe(environmentID, resourceType, recipeName)
    CfgLoader-->>Eng: EnvironmentDefinition (Driver, TemplatePath, ...)
    Eng->>Eng: lookup driver by TemplateKind
    Eng->>Drv: driver.Execute
    Drv->>Infra: provision (Bicep via deployment engine / Terraform via tf binary)
    Drv-->>Eng: RecipeOutput (resources, values, secrets)
    Eng-->>RecipeCtrl: RecipeOutput
    RecipeCtrl->>DB: apply recipe output and save updated resource/status
  end
```

### How UCP Routes The Request

[pkg/ucp/frontend/controller/radius/proxy.go](../../pkg/ucp/frontend/controller/radius/proxy.go)
implements `ProxyController.Run`. For every request to a Radius-plane resource
it calls
[resourcegroups.ValidateDownstream](../../pkg/ucp/frontend/controller/resourcegroups/util.go),
which loads the `Location` for the resource type and reads
`location.Properties.Address`. If the address is set, UCP proxies to that URL;
otherwise it falls back to the `defaultDownstream` configured at startup,
which in practice is dynamic-rp ([pkg/ucp/config.go](../../pkg/ucp/config.go)).

This is the extension point that allows both dynamic and dedicated resource
providers. A location with no address uses dynamic-rp; a location with an
address routes to the RP implementation at that URL. UCP still validates the
registered type and API version in both cases before forwarding the request.

After proxying a successful mutating request, UCP may also update tracked
resource state. If the downstream response is terminal it tries to update the
tracked resource synchronously; otherwise it queues a background tracked
resource update. This is separate from the dynamic-rp recipe operation queue.

### How dynamic-rp Picks A Path

[pkg/dynamicrp/backend/controller/dynamicresource.go](../../pkg/dynamicrp/backend/controller/dynamicresource.go)
hosts the generic async controller. It:

1. Looks up the `ResourceType` / `APIVersion` and validates the incoming body
   against the schema.
2. Reads the type's capabilities and the operation method (PUT / DELETE).
3. Returns an inert controller when the type declares
   `ManualResourceProvisioning` (resources whose state is provided by the
   caller, not provisioned by a recipe), or a recipe-backed controller
   otherwise:
   - PUT → [putrecipe.go](../../pkg/dynamicrp/backend/controller/putrecipe.go) →
     `portableresources/backend/controller.NewCreateOrUpdateResource`
   - DELETE → [deleterecipe.go](../../pkg/dynamicrp/backend/controller/deleterecipe.go)

The dynamic-rp frontend handles the first half of the async operation. Its PUT
route uses `defaultoperation.NewDefaultAsyncPut`, which converts the request,
runs update filters, saves the accepted resource, queues the async operation,
and returns the ARM async response. The backend worker later dispatches that
queued operation to the default controller registered in
[pkg/dynamicrp/backend/service.go](../../pkg/dynamicrp/backend/service.go).

Dynamic resources also use schema annotations for sensitive input handling.
The frontend encryption filter fetches the resource schema, finds fields marked
with `x-radius-sensitive`, and encrypts those property values before the
accepted resource is stored. During backend processing,
`CreateOrUpdateResource` decrypts a recipe-only copy of those values, persists
a redacted resource, and passes the decrypted copy to the recipe engine. This
keeps recipe input usable without storing sensitive plaintext.

### How The Recipe Runs

[pkg/portableresources/backend/controller/createorupdateresource.go](../../pkg/portableresources/backend/controller/createorupdateresource.go)
assembles a `recipes.ResourceMetadata` (environment ID, application ID, the
resource's own ID and properties, and any connected-resource metadata) and
calls `engine.Execute`.

The engine
([pkg/recipes/engine/engine.go](../../pkg/recipes/engine/engine.go)) then:

1. Loads runtime configuration for the environment via
   [`configloader.LoadConfiguration`](../../pkg/recipes/configloader/environment.go).
2. Skips driver execution when the environment is simulated.
3. Loads the recipe definition via
   [`configloader.LoadRecipe`](../../pkg/recipes/configloader/environment.go),
   which fetches the environment, looks up
   `Properties.Recipes[resourceType][recipeName]`, and returns an
   `EnvironmentDefinition` whose `Driver` field is the `TemplateKind` string.
4. Selects the driver from the engine's `Drivers` map keyed by template kind
   (see
   [pkg/recipes/controllerconfig/config.go](../../pkg/recipes/controllerconfig/config.go) —
   `recipes.TemplateKindBicep` maps to the bicep driver, `TemplateKindTerraform`
   to the terraform driver).
5. Loads any driver-required secrets through `DriverWithSecrets` and the
  environment's configured secret stores.
6. Calls `driver.Execute`. The bicep driver hands the template to the
   deployment engine; the terraform driver shells out to the terraform binary.
7. Returns a `RecipeOutput` (`Resources`, `Values`, `Secrets`) to the
  controller.

The dynamic resource processor then validates the output, records deployed
resources and values under status, and copies computed or secret values back
into top-level resource properties only when those property names are present
in the registered schema. This schema filter prevents recipe output from
inventing arbitrary user-visible properties.

## Delete Invocation

DELETE follows the same UCP routing, dynamic-rp frontend, async queue, and
`DynamicResourceController` selection path as PUT. The recipe-backed branch
delegates to [deleterecipe.go](../../pkg/dynamicrp/backend/controller/deleterecipe.go),
which constructs the shared `DeleteResource` controller.

```mermaid
sequenceDiagram
  participant UCP as UCP ProxyController
  participant DynFE as dynamic-rp frontend
  participant Async as Status manager / Queue
  participant Worker as dynamic-rp backend worker
  participant DynCtrl as DynamicResourceController
  participant DeleteCtrl as RecipeDeleteController / DeleteResource
  participant Eng as Recipe engine
  participant Drv as bicep or terraform driver
  participant DB as Database

  UCP->>DynFE: proxy DELETE
  DynFE->>Async: queue async DELETE operation
  DynFE-->>UCP: ARM async response
  Async->>Worker: dequeue operation
  Worker->>DynCtrl: dispatch to default controller
  DynCtrl->>DynCtrl: select controller from capabilities
  alt ManualResourceProvisioning
    DynCtrl->>DB: NewInertDeleteController deletes resource record
  else default (recipe-backed)
    DynCtrl->>DeleteCtrl: NewRecipeDeleteController
    DeleteCtrl->>DB: load stored resource and output resources
    DeleteCtrl->>Eng: Delete(ResourceMetadata, OutputResources)
    Eng->>Drv: driver.Delete
    Drv-->>Eng: deletion complete
    DeleteCtrl->>DB: delete resource record
  end
```

`DeleteResource` skips driver deletion when the recipe deployment failed during
setup, because no output resources were created. Otherwise it passes the stored
output resources to the driver so recipe-created infrastructure can be cleaned
up before the Radius resource record is removed.

## Invariants

- The contract between UCP and any RP (built-in or dynamic) is `Location`
  routing on the resource type — never type-specific switching in UCP.
- dynamic-rp must remain type-agnostic. Schema validation and capability
  inspection drive behavior; there is no per-type code path here.
- Recipes are owned by the environment, not the type. A type with no recipe
  registered for the environment fails at `LoadRecipe` with
  `RecipeNotFoundFailure`.
- Dynamic resources default to recipe name `default` when `properties.recipe`
  is omitted.
- The driver lookup key is the recipe's `templateKind`. Adding a new driver
  means registering it in the engine's `Drivers` map.
- Recipe output only becomes user-visible resource properties when those
  properties exist in the registered schema.
- Sensitive input fields marked by schema annotations are encrypted at rest and
  decrypted only for in-memory recipe execution.

## Change This Safely

- Adding a new built-in resource type: add a manifest, register it through the
  CLI or initializer, and (if it has a default behavior) wire a recipe in the
  environment Bicep used by tests.
- Adding a new recipe driver: implement `recipes/driver.Driver`, register it
  in [controllerconfig/config.go](../../pkg/recipes/controllerconfig/config.go),
  and pick a stable `TemplateKind` string used by manifests and the CLI.
- Adding a new capability that changes the deployment path: extend
  `DynamicResourceController.selectController` and add the corresponding
  controller under
  [pkg/dynamicrp/backend/controller](../../pkg/dynamicrp/backend/controller/).
- Changing the manifest schema: update
  [pkg/cli/manifest](../../pkg/cli/manifest/) **and** the initializer's direct
  database writer in [pkg/ucp/initializer/service.go](../../pkg/ucp/initializer/service.go).
  Both paths must produce equivalent records.

## Related Docs

- [dynamic-rp.md](dynamic-rp.md) — the process that hosts the generic
  controllers described above.
- [ucp.md](ucp.md) — UCP request routing and the proxy controller.
- [deployment-engine.md](deployment-engine.md) — what the bicep driver hands
  templates to.
- [terraform-bicep-config.md](terraform-bicep-config.md) — environment-level
  recipe configuration (private registries, provider installation, env vars).
- [state-persistence.md](state-persistence.md) — where resource and recipe
  state lives.
