---
type: docs
title: "Project Radius"
linkTitle: "Home"
description: "Developer-centric cloud-native application platform"
weight: 1
---

<img src="azure-app.png" alt="Screenshot of an Azure application" style="width: 100%; max-width: 1200px;">

## Getting started

Author and deploy your first appliction in minutes with Radius:

{{< button text="Install Radius 🚀" page="install-cli.md" color="success" >}}

## Features

{{< cardpane width=1200px >}}

{{< card header="**📃 Application model**" title="Descriptive framework for cloud-native applications" >}}
  Model your Radius Application using **Components**, **Connections**, **Traits**, and **Scopes** which describe the functionality of your app, all in the Bicep language.

  <pre style="color:#f8f8f2;background-color:#272822;-moz-tab-size:4;-o-tab-size:4;tab-size:4">resource app <span style="color:#e6db74">'Application'</span> <span style="color:#f92672">=</span> <span style="color:#f92672">{</span>
  name: <span style="color:#e6db74">'eShop'</span>

  resource basket <span style="color:#e6db74">'ContainerComponent'</span> <span style="color:#f92672">=</span> <span style="color:#f92672">{</span>...<span style="color:#f92672">}</span>
  resource orders <span style="color:#e6db74">'ContainerComponent'</span> <span style="color:#f92672">=</span> <span style="color:#f92672">{</span>...<span style="color:#f92672">}</span>
  resource inventory <span style="color:#e6db74">'dapr.StateStoreComponent'</span> <span style="color:#f92672">=</span> <span style="color:#f92672">{</span>...<span style="color:#f92672">}</span>
  ...<span style="color:#f92672">
}</span></pre>
  
  [**Learn the Radius app model**]({{< ref appmodel-concept >}})
{{< /card >}}

{{< card header="**🔌 Portable components**" title="Deploy your applications to cloud and edge with zero code changes" >}}
  Radius Components describe the code, data, and infrastructure pieces of an application. They capture behavior and requirements, and make it easy to parameterize and deploy across platforms.
  
  Portable Components, like [Dapr]({{< ref dapr >}}), can be deployed to different Radius platforms with no changes to your code.

  <table style="max-width:600px">
  <tr>
    <td style="width:25%;text-align:center">
      <img src="dapr-logo.svg" alt="Dapr logo" style="width:80%">
    </td>
    <td style="width:25%;text-align:center">
      <img src="mongo-logo.png" alt="Mongo logo" style="width:80%">
    </td>
    <td style="width:25%;text-align:center">
      <img src="redis-logo.png" alt="Redis logo" style="width:80%">
    </td>
    <td style="width:25%;text-align:center">
      <img src="rabbitmq-logo.png" alt="RabbitMQ logo" style="width:80%">
    </td>
  </tr>
  </table>
  <br />
  
  [**Check out the Radius components**]({{< ref appmodel-concept >}})
{{< /card >}}

{{< /cardpane >}}
{{< cardpane width=1200px >}}

{{< card header="**☁ Managed environments**" title="Descriptive framework for cloud-native applications" >}}
  A Radius environment is where you deploy and host Radius applications.
  
  It includes a **control-plane**, which communicates with with the rad CLI, and a **runtime** to which applications are deployed.

  Radius offers support for both cloud and edge with [Kubernetes]({{< ref kubernetes >}}) and [Azure]({{< ref azure >}}) managed environments.
  
  For development, [local]({{< ref local >}}) environments allow you to run Radius applications locally.

  <table style="max-width:600px">
  <tr>
    <td style="width:50%;text-align:center">
      <a href="{{< ref kubernetes >}}"><img src="platforms/kubernetes-logo.svg" alt="Kubernetes logo" style="width:80%"></a>
    </td>
    <td style="width:50%;text-align:center">
      <a href="{{< ref azure >}}"><img src="platforms/azure-logo.png" alt="Azure logo" style="width:80%"></a>
    </td>
  </tr>
  <tr>
    <td colspan="2" style="width:100%;text-align:center">
      <a href="{{< ref local >}}"><img src="platforms/local-logo.png" alt="Local logo" style="width:70%"></a>
    </td>
  </tr>
  </table>
  <br />
  
  [**Initialize Radius on your platform**]({{< ref platforms >}})
{{< /card >}}

{{< card header="**⌨ rad CLI**" title="Easily initialize environments and deploy Radius applications" >}}
  The rad CLI is your primary interface with Radius [environments]({{< ref environments-concept >}}) and [applications]({{< ref appmodel-concept >}}).

  Developers can initialize environments across platforms, deploy applications, view logs, check status, and more.

  <pre style="color:#f8f8f2;background-color:#272822;-moz-tab-size:4;-o-tab-size:4;tab-size:4">
  $ rad env init Azure
  Initializing Azure environment...
  $ rad deploy eshop.bicep
  Deploying application 'eShop' into environment 'Azure'...
  $ rad resource logs -c orders -a eshop
  Order #1 received
  Order #2 received
  ...</pre>
  
  
  [**Install the rad CLI**]({{< ref install-cli.md >}})
{{< /card >}}

{{< /cardpane >}}
