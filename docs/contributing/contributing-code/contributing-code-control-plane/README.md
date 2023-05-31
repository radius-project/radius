# Contributing to the Radius control-plane

The Radius control-plane services are the microservices that run to deploy and manage applications and cloud resources. This page is an index of relevant topics for developing with the control-plane.

You might hear these components referred to as:

- UCP (Universal control plane): Front-door proxy and integration with cloud resources
- Core RP (`Applications.Core` Resource Provider): Support for core Radius concepts like applications and containers
- Dapr RP (`Applications.Dapr` Resource Provider): Support for Dapr integration
- Datastores RP (`Applications.Datastores` Resource Provider): Support for databases and recipes
- Messaging RP (`Applications.Messaging` Resource Provider): Support for messaging technologies and recipes
- Link RP: Legacy name for Dapr, Datastores, or Messaging RP

## Table of contents

- [Configuration](./configSettings.md)
- [Logging](logging.md)
- [Running the control-plane locally](./running-controlplane-locally.md)
