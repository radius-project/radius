---
type: docs
title: "Radius Local Environment Tutorial"
linkTitle: "Radius local environment microservices tutorial"
description: "Learn Project Radius by authoring templates and deploying a working application made up of microservices locally"
weight: 200
no_list: true
---

## Overview

This tutorial will teach you how to deploy  a sample microservice solution for Azure Container Apps using Radius locally. You will learn:  

- The concepts of the Radius application model 
- How Radius can interact with multiple other frameworks such as Dapr and Docker 
- The basic syntax of the Bicep language 

No prior knowledge of Dapr, Radius, Docker, or Bicep is needed.

### Tutorial Steps
In this tutorial, you will:
1. Review and understand the Radius container app store microservice application
1. Initialize your Radius environment
1. Understand a basic overview of the rad cli abstractions
1. Run your application through the Radius Cli

## Prerequisites

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Install Dapr](https://docs.dapr.io/getting-started/install-dapr-cli/)
- [(optional) Setup Visual Studio Code]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) for syntax highlighting, completion, and linting.
   - You can also complete this tutorial with any basic text editor.

<br>{{< button text="Application overview" page="local-overview.md" >}}