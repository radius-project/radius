---
type: docs
title: "Dapr Microservices Tutorial"
linkTitle: "Dapr microservices"
description: "Learn Project Radius by authoring templates and deploying a Dapr application"
weight: 200
no_list: true
---

## Overview

This tutorial will teach you how to deploy a Dapr microservices application using Radius. You will learn:  

- The concepts of the Radius application model
- How Dapr and Radius seamlessly work together  
- The basic syntax of the Bicep language

No prior knowledge of Dapr, Radius, or Bicep is needed.

### Tutorial Steps

In this tutorial, you will:

1. Review and understand the Radius Dapr microservices application
1. Deploy the frontend code in a container
1. Connect a Dapr statestore resource
1. Connect a python-based order generator  

## Prerequisites

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create a Radius environment]({{< ref create-environment.md >}})
- [(optional) Setup Visual Studio Code]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) for syntax highlighting, completion, and linting.
  - You can also complete this tutorial with any basic text editor.

{{% alert title="Dapr initialization" color="warning" %}}
If you are runnning this tutorial on Kubernetes, make sure to manually [initialize Dapr via the dapr CLI](https://docs.dapr.io/operations/hosting/kubernetes/kubernetes-deploy/#install-dapr). This behavior will be automated in an upcoming release.
{{% /alert %}}

<br />
{{< button text="Application overview" page="dapr-microservices-overview.md" >}}
