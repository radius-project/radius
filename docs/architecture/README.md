# Service Architecture

This folder contains living architecture documentation for the executables and
shared runtime patterns implemented in this repository.

These docs are for both human contributors and AI agents. Each primary service
page is code-oriented and includes the entry points, the packages that matter
most, one package dependency view, one representative flow, and focused
change-safety guidance.

## Start Here

- [service-interaction-map.md](service-interaction-map.md) explains how the main
  binaries fit together.
- [shared-runtime-and-armrpc.md](shared-runtime-and-armrpc.md) explains the
  common hosting, HTTP, builder, and async-operation framework used across the
  services.
- [ucp.md](ucp.md) explains how UCP routes and adapts requests.
- [dynamic-rp.md](dynamic-rp.md) explains the generic resource provider used for
  authoring and handling Radius resource types.
- [extensibility.md](extensibility.md) explains how resource types and recipes
  are registered and how they are invoked during deployment.
- [deployment-engine.md](deployment-engine.md) explains the deployment engine
  that processes Bicep/ARM deployments.
- [controller.md](controller.md) explains the Kubernetes controller process and
  its reconcilers.
- [rad-cli.md](rad-cli.md) explains how the CLI is wired and how commands reach
  backend services.
- [state-persistence.md](state-persistence.md) explains the shared database,
  secret, and queue abstractions used by the control-plane services.
- [credentials.md](credentials.md) explains how cloud credentials are stored
  and used for deployments, and how clients authenticate to a Radius install.
- [application-graph.md](application-graph.md) explains how the application
  graph is computed from stored resources and displayed via the CLI.
- [terraform-bicep-config.md](terraform-bicep-config.md) explains the reusable
  `Radius.Core/terraformConfigs` and `bicepConfigs` resources referenced by
  environments to provide private registry auth, Terraform CLI provider
  installation rules, and recipe environment variables.

## Reading Order

If you are new to the codebase, read these in order:

1. [service-interaction-map.md](service-interaction-map.md)
2. [shared-runtime-and-armrpc.md](shared-runtime-and-armrpc.md)
3. [ucp.md](ucp.md)
4. [dynamic-rp.md](dynamic-rp.md)
5. [extensibility.md](extensibility.md)
6. [deployment-engine.md](deployment-engine.md)
7. [controller.md](controller.md)
8. [rad-cli.md](rad-cli.md)
9. [state-persistence.md](state-persistence.md)
10. [credentials.md](credentials.md)
11. [application-graph.md](application-graph.md)

## Related Material

- UCP-specific background and older walkthroughs live in [../ucp](../ucp).
- contributor-oriented operational setup lives in
  [../contributing](../contributing).
- larger design proposals and historical decisions live in the external
  `radius-project/design-notes` repository.
