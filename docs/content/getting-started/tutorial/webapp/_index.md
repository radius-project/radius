---
type: docs
title: "Website tutorial"
linkTitle: "Website"
description: "Learn Project Radius by authoring templates and deploying a working website with a database."
weight: 100
no_list: true
---

## Overview

This tutorial will teach you how to deploy a website as a Radius application from first principles. You will learn:  

- The concepts of the Radius application model 
- The basic syntax of the Bicep language 

No prior knowledge of Radius or Bicep is needed.

### Tutorial Steps
In this tutorial, you will:
1. Review and understand the Radius website application
1. Deploy the website frontend in a container
1. Connect a CosmosDB resource to your website application

## Prerequisites

- [Install Radius CLI]({{< ref install-cli.md >}})
- [Create a Radius environment]({{< ref create-environment.md >}})
- [(recommended) Install Visual Studio Code](https://code.visualstudio.com/)
   - The [Radius VSCode extension]({{< ref "install-cli.md#2-install-custom-vscode-extension" >}}) provides syntax highlighting, completion, and linting.
   - You can also complete this tutorial with any basic text editor.

{{< button text="Application overview" page="webapp-overview.md" >}}