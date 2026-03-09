# Contributing to the Radius control-plane

The Radius control-plane services are the microservices that run to deploy and manage applications and cloud resources. This page is an index of relevant topics for developing with the control-plane.

If you need code-oriented architecture context before changing the services, see
the architecture set in [../../../architecture/README.md](../../../architecture/README.md).
The most relevant pages are the service interaction map, shared runtime,
service-specific architecture docs, and the call-chain walkthroughs.

You might hear these components referred to as:

- UCP (Universal control plane): Front-door proxy and integration with cloud resources
- Core RP (`Applications.Core` Resource Provider): Support for core Radius concepts like applications and containers
- Dapr RP (`Applications.Dapr` Resource Provider): Support for Dapr integration
- Datastores RP (`Applications.Datastores` Resource Provider): Support for databases and recipes
- Messaging RP (`Applications.Messaging` Resource Provider): Support for messaging technologies and recipes
- Link RP: Legacy name for Dapr, Datastores, or Messaging RP

## Table of contents

- [Architecture docs](../../../architecture/README.md)
- [Configuration](./configSettings.md)
- [Logging](logging.md)
- [Running the control-plane locally](./running-controlplane-locally.md)
- [Generating and installing a custom build](./generating-and-installing-custom-build.md)
- [Troubleshooting the installation](./troubleshooting-installation.md)

