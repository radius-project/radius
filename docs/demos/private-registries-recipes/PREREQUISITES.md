# Prerequisites: Setting up private registries for the demo

This guide covers the **high-level steps to stand up the private registries** used
by the [Private Registries & Repositories demo](./README.md). The demo proves that
Radius can pull recipes from registries that require authentication, so you need
**at least one private registry you control** for each scenario you want to run:

- **Scenario 1 (Bicep):** a private **OCI** registry that hosts Bicep artifacts.
- **Scenario 2 (Terraform):** a private **Terraform** module registry/repository
  that authenticates with a token.
- **Scenario 3 (Combined):** both of the above.

You do **not** need every provider listed below - pick whichever one you already
have access to. Each section is a short, provider-agnostic checklist plus one
concrete example. Once a registry is ready, return to the
[demo README](./README.md) and supply the values it produced.

> These steps are for **demoing/testing only**. They favor the fastest path to a
> working private registry, not production hardening. Use throwaway/scoped
> credentials and tear them down afterward (see [Cleanup](#cleanup)).

---

## Base tooling

Before setting up any registry, make sure you have:

1. A Kubernetes cluster and `kubectl` configured to talk to it.
2. The [`rad` CLI](https://docs.radapp.io/installation/) installed
   (`rad version` works). The Bicep tooling for `rad bicep publish` is bundled.
3. Radius installed on the cluster with the `Radius.Core` resource types
   available (`rad install kubernetes`).
4. The CLI for whichever registry provider you choose (e.g. `az`, `gh`, `aws`,
   or `terraform`).

---

## Part A - Private Bicep (OCI) registry

**Goal:** a private OCI registry you can `rad bicep publish` to, and a
username/password pair the cluster can use to pull from it (the demo's Bicep
scenarios use `BasicAuth`).

### High-level steps (any OCI provider)

1. **Create a private OCI registry.** Any registry that speaks the OCI artifact
   spec works - Azure Container Registry (ACR), GitHub Container Registry (GHCR),
   Amazon ECR, Google Artifact Registry, Docker Hub (private repo), Harbor, etc.
   Ensure it is **private** (not anonymously pullable) so the demo actually
   exercises authentication.
2. **Create scoped pull credentials.** Generate a username + password/token that
   has at least **pull** rights on the recipe repository (and **push** rights if
   the *same* identity will publish the recipe). Prefer a scoped token over an
   admin account.
3. **Authenticate your local tooling once** so you can push the recipe artifact
   (e.g. `az acr login`, `docker login`, `gh auth token | docker login`).
4. **Record these values** - you'll pass them to the demo:
   - `BICEP_REGISTRY` - the registry hostname, e.g. `myregistry.azurecr.io`.
   - `BICEP_RECIPE` - the full artifact reference, e.g.
     `myregistry.azurecr.io/recipes/redis:latest`.
   - `BICEP_REGISTRY_USERNAME` / `BICEP_REGISTRY_PASSWORD` - the pull credentials.

### Concrete example - Azure Container Registry (ACR)

```bash
# 1. Create a private ACR (Basic SKU is fine for a demo).
az group create --name radius-demo-rg --location eastus
az acr create --resource-group radius-demo-rg --name myregistry --sku Basic

# 2. Create a scoped, repository-limited token for pulling the recipe.
az acr token create \
  --registry myregistry \
  --name radius-demo-pull \
  --repository recipes/redis content/read content/write

# The command prints a token password - capture it now (it is shown once).

# 3. Log in locally so you can publish the recipe.
az acr login --name myregistry
```

Then record:

```bash
export BICEP_REGISTRY="myregistry.azurecr.io"
export BICEP_RECIPE="${BICEP_REGISTRY}/recipes/redis:latest"
export BICEP_REGISTRY_USERNAME="radius-demo-pull"
export BICEP_REGISTRY_PASSWORD="<token-password-from-step-2>"
```

### Concrete example - GitHub Container Registry (GHCR)

```bash
# 1/2. Create a classic PAT with write:packages + read:packages scope at
#      https://github.com/settings/tokens, then log in.
echo "<your-pat>" | docker login ghcr.io -u <your-github-username> --password-stdin
```

```bash
export BICEP_REGISTRY="ghcr.io/<your-github-username>"
export BICEP_RECIPE="${BICEP_REGISTRY}/recipes/redis:latest"
export BICEP_REGISTRY_USERNAME="<your-github-username>"
export BICEP_REGISTRY_PASSWORD="<your-pat>"
```

> GHCR packages are private by default. Leave the package private so the demo
> exercises authentication.

The demo's [Scenario 1](./README.md#scenario-1--private-bicep-recipe-registry-oci)
publishes [`recipes/redis-recipe.bicep`](./recipes/redis-recipe.bicep) to
`BICEP_RECIPE` and feeds the credentials into a `Radius.Core/bicepConfigs`.

---

## Part B - Private Terraform module registry / repository

**Goal:** a private Terraform module source that authenticates with a **token**,
plus the module address Radius should fetch. The demo's
`Radius.Core/terraformConfigs` renders a `.terraformrc` `credentials` block for
that host.

### High-level steps (any token-authenticated registry)

1. **Choose a private module source** that authenticates over HTTPS with a bearer
   token. You have two broad options:
   - **Cloud / hosted (official):** Terraform Cloud / HCP Terraform
     (`app.terraform.io`).
   - **Self-hosted OSS:** run your own private module registry. Lightweight,
     open-source options implement the
     [Terraform Module Registry protocol](https://developer.hashicorp.com/terraform/internals/module-registry-protocol)
     and authenticate with a token - for example
     [Terralist](https://www.terralist.io/) (also acts as a provider registry),
     or an Artifactory/Harbor Terraform registry. These are a good fit when you
     want a fully private registry without a SaaS account.
2. **Publish (or identify) a module** that provisions the demo resource - a
   Kubernetes Redis cache matches the sample. Note its full module address.
3. **Create an API token** with permission to read the module from that registry.
4. **Record these values** - you'll pass them to the demo:
   - `TF_REGISTRY_HOST` - the registry hostname, e.g. `app.terraform.io`.
   - `TF_RECIPE_LOCATION` - the module source address, e.g.
     `app.terraform.io/my-org/redis/kubernetes`.
   - `TF_REGISTRY_TOKEN` - the API token.

### Concrete example - HCP Terraform (Terraform Cloud)

1. Sign in at <https://app.terraform.io> and create (or use) an organization.
2. Publish a private module to the organization's **Registry** (for a demo, you
   can publish from a Git repo containing a small Kubernetes Redis module, or use
   an existing private module).
3. Create a token under **User settings → Tokens** (or a team/organization token).

```bash
export TF_REGISTRY_HOST="app.terraform.io"
export TF_RECIPE_LOCATION="app.terraform.io/my-org/redis/kubernetes"
export TF_REGISTRY_TOKEN="<your-terraform-registry-token>"
```

### Concrete example - self-hosted OSS registry ([Terralist](https://www.terralist.io/))

For a fully private setup without a SaaS account, run an open-source registry
such as [Terralist](https://www.terralist.io/), which implements the Terraform
module (and provider) registry protocol and supports token-based auth.

1. **Deploy Terralist** following its
   [documentation](https://www.terralist.io/docs/) (it ships as a single binary /
   container and uses a SQL database plus object storage for module/provider
   artifacts). Expose it over HTTPS at a hostname you control, e.g.
   `registry.mycompany.com`.
2. **Publish a module** (a small Kubernetes Redis module works for the demo) to
   your Terralist instance and note its address.
3. **Create an API token** in Terralist for Radius to pull the module.

```bash
export TF_REGISTRY_HOST="registry.mycompany.com"
export TF_RECIPE_LOCATION="registry.mycompany.com/my-org/redis/kubernetes"
export TF_REGISTRY_TOKEN="<your-terralist-api-token>"
```

> Any registry that implements the Terraform module registry protocol and
> authenticates with a token works the same way - Radius only needs the host, the
> module address, and the token.

> `TF_RECIPE_LOCATION` is whatever module source your private recipe lives at - a
> private registry module address or an HTTP module archive URL. The demo's
> [Scenario 2](./README.md#scenario-2--private-terraform-module-registry--repository)
> stores the token in a `SecretStore` and references it from a
> `Radius.Core/terraformConfigs`.

> **Git-based private modules (PAT auth):** authenticating to a private **Git**
> module source with a personal access token is a separate path that the new
> `Radius.Core/terraformConfigs` resource does not cover yet. For Git PAT auth
> today, use the legacy `Applications.Core/environments` `recipeConfig` path -
> see [Known limitations](./README.md#known-limitations-as-of-this-demo).

---

## Verify and continue

Once your registries are ready and the variables are set, verify the base setup
and continue with the [demo README](./README.md):

```bash
rad version
kubectl get nodes
```

The README's [Prerequisites](./README.md#prerequisites) section creates the
Radius group and namespaces; its scenarios then publish the recipe and deploy
using the values you recorded above. The
[automated E2E runner](./README.md#automated-e2e-runner) reads the **same**
environment variables.

---

## Cleanup

Tear down the throwaway registries and credentials when you're done:

- **ACR:** `az group delete --name radius-demo-rg --yes --no-wait`
  (or just `az acr token delete --registry myregistry --name radius-demo-pull`).
- **GHCR:** delete the package from the repository/org **Packages** UI and revoke
  the PAT at <https://github.com/settings/tokens>.
- **HCP Terraform:** delete the module from the registry and revoke the token in
  **User settings → Tokens**.
- **Self-hosted (e.g. Terralist):** revoke the API token and remove the module
  (and the instance itself if it was stood up only for the demo).

See the demo README's [Cleanup](./README.md#cleanup) section to remove the Radius
applications, group, and namespaces.
