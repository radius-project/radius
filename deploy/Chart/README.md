## Introduction

The Radius helm chart deploys the Radius control plane system services on a Kubernetes cluster using Helm. For more information on installing Radius to Kubernetes refer to the [Radius docs](https://docs.radapp.dev/operations/platforms/kubernetes/kubernetes-install/#install-with-helm).

### Chart Details

This chart installs the Radius controller manager, which includes:

- CRDs (Custom Resource Definitions) for Radius Applications, Components, Deployments, etc.
- Corresponding controllers for the CRDs, which will act on resources being created, updated, or deleted in the cluster.
- Validating Webhooks, which will validate the CRDs before going to the controller.

## Installation 

Visit the [Radius docs](https://docs.radapp.dev/operations/platforms/kubernetes/kubernetes-install/#install-with-helm) to learn how to install and configure Radius on Kubernetes using the Radius Helm chart.
