# Contributing to the Radius control-plane

The Radius control-plane services are the microservices that run to deploy and manage applications and cloud resources. This page is an index of relevant topics for developing with the control-plane.

If you need code-oriented architecture context before changing the services, see the architecture docs in [../../../architecture/README.md](../../../architecture/README.md).
The most relevant pages are the service interaction map, shared runtime, UCP,
`dynamic-rp`, controller, and CLI architecture docs.

For new authoring work in this repo, prefer Radius resource types and generic
provider behavior in `dynamic-rp` over legacy `Applications.*` resource work.

You might hear these components referred to as:

- UCP (Universal control plane): Front-door proxy and integration with cloud resources
- Dynamic RP: Main authoring surface for Radius resource types and generic resource behavior
- Legacy Applications.* RPs: Older provider processes that still exist in the runtime but are not the preferred target for new authoring work

## Table of contents

- [Architecture docs](../../../architecture/README.md)
- [Configuration](./configSettings.md)
- [Logging](logging.md)
- [Running the control-plane locally](./running-controlplane-locally.md)
- [Generating and installing a custom build](./generating-and-installing-custom-build.md)
- [Troubleshooting the installation](./troubleshooting-installation.md)

