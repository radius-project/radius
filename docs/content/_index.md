---
type: docs
title: "Project Radius"
linkTitle: "Home"
description: "Applications for developers"
weight: 1
---

<img src="azure-app.png" alt="Screenshot of an Azure application" style="width: 70%;">

## Overview

Project Radius lets you model, deploy, and manage applications across cloud and edge. It is designed to be:

{{< cardpane >}}
{{< card header="**App-focused**">}}
Manage your applications as individual resources instead of a list of infrastructure.
<img src="radius-app-list.png" alt="Screenshot of a list of Radius applications in the Azure portal" style="width:600px" >
{{< /card >}}
{{< card header="**Portable**" >}}
Deploy your application across cloud and edge with support for both [Microsoft Azure]({{< ref azure >}}) and [Kubernetes]({{< ref kubernetes >}}) platforms.
<table style="max-width:600px;margin-top:10%">
  <tr>
    <td style="width:50%;text-align:center">
      <a href="{{< ref azure >}}"><img src="platforms/azure-logo.png" alt="Azure logo" style="width:80%"></a>
    </td>
    <td style="width:50%;text-align:center">
      <a href="{{< ref kubernetes >}}"><img src="platforms/kubernetes-logo.svg" alt="Kubernetes logo" style="width:80%"></a>
    </td>
  </tr>
</table>
{{< /card >}}
{{< /cardpane >}}
{{< cardpane >}}
{{< card header="**Productive**" >}}
Leverage the Bicep language and set of tooling to build your model and deploy your application.
<table style="max-width:600px">
  <tr>
    <td style="width:50%;text-align:center">
      <a href="https://github.com/Azure/Bicep" target="_blank"><img src="bicep-logo.png" alt="Bicep logo" style="width:70%"></a>
    </td>
    <td style="width:50%;text-align:center">
      <a href="{{< ref setup-vscode >}}"><img src="vscode-logo.png" alt="Visual Studio Code logo" style="width:80%"></a>
    </td>
  </tr>
</table>
{{< /card >}}
{{< card header="**Open**" >}}
Radius resources are extensible, allowing you to add your own resource types and customizations.
<table style="max-width:600px;margin-top:5%">
  <tr>
    <td style="width:25%;text-align:center">
      <a href="{{< ref dapr >}}"><img src="dapr-logo.svg" alt="Dapr logo" style="width:50%"></a>
    </td>
    <td style="width:25%;text-align:center">
      <a href="{{< ref mongodb >}}"><img src="mongo-logo.png" alt="Mongo logo" style="width:50%"></a>
    </td>
    <td style="width:25%;text-align:center">
      <a href="{{< ref redis >}}"><img src="redis-logo.png" alt="Redis logo" style="width:50%"></a>
    </td>
    <td style="width:25%;text-align:center">
      <a href="{{< ref rabbitmq >}}"><img src="rabbitmq-logo.png" alt="RabbitMQ logo" style="width:50%"></a>
    </td>
  </tr>
  </table>
{{< /card >}}
{{< /cardpane >}}


## Getting started

Author and deploy your first appliction in minutes with Radius:

{{< button text="Install Radius 🚀" page="install-cli.md" color="success" >}}
