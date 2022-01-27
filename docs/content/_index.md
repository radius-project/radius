---
type: docs
title: "Project Radius"
linkTitle: "Home"
description: "Model, deploy, and manage applications across cloud and edge"
weight: 1
---

<img src="azure-app.png" alt="Screenshot of an Azure application" style="width: 70%;">

{{< cardpane >}}
{{< card header="**App-centric**" >}}
Developers [describe their application]({{< ref appmodel-concept >}}) services and relationships, rather than just a list of infrastructure.

<img src="app-diagram.png" alt="Screenshot of a Radius applications diagram" style="width:100%" >
{{< /card >}}
{{< card header="**Portable**" >}}
Radius applications and tooling are agnostic of platform, services, and infrastructure. 
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
Radius extends strong existing tools to further streamline the app developer experience. 
<table style="max-width:600px">
  <tr>
    <td style="width:50%;text-align:center">
      <a href="{{< ref setup-vscode >}}"><img src="vscode-logo.png" alt="Visual Studio Code logo" style="width:80%"></a>
    </td>
    <td style="width:50%;text-align:center">
      <a href="https://github.com/Azure/Bicep" target="_blank"><img src="bicep-logo.png" alt="Bicep logo" style="width:70%"></a>
    </td>
  </tr>
</table>
{{< /card >}}
{{< card header="**Intelligent**" >}}
Developers can offload the complexity of wiring-up applications and let Radius employ best-practices.
<table style="max-width:600px;margin-top:5%">
  <tr>
    <td style="width:25%;text-align:center">
      <a href="{{< ref connections-model >}}"><img src="connect-logo.svg" alt="Connections logo" style="width:40%"></a>
    </td>
    <td style="width:25%;text-align:center">
      <a href="{{< ref networking >}}"><img src="network-logo.svg" alt="Networking logo" style="width:40%"></a>
    </td>
    <td style="width:25%;text-align:center">
      <a href="{{< ref dapr >}}"><img src="dapr-logo.svg" alt="Dapr logo" style="width:40%"></a>
    </td>
  </tr>
  </table>
{{< /card >}}
{{< /cardpane >}}

{{< button text="Get started with Radius 🚀" page="install-cli.md" color="success" size="btn-lg" >}}
