---
type: docs
title: "Project Radius Documentation"
linkTitle: "Project Radius docs"
description: "Documentation on the Project Radius application model"
weight: 1
---

Project Radius is a developer-centric cloud-native application platform.

<iframe width="640" height="360" src="https://msit.microsoftstream.com/embed/video/d2dd0840-98dc-94b1-0221-f1eb96448a8e?autoplay=false&showinfo=true" allowfullscreen style="border:none;"></iframe>

## Getting started

You can begin with Radius by downloading and installing the Radius CLI

{{< button text="Install Radius 🚀" page="install-cli.md" color="success" >}}

## Radius features

{{< cardpane >}}

{{< card header="**Model application behavior**" title="Describe your apps by what they consume and provide, instead of what they run on" >}}
  The Radius application model allows you to describe the services, dependencies, and traits your app provides.

  Developers no longer need to describe the infrastructure and connections that make up the underlying platform.
{{< /card >}}

{{< card header="**Automate best practices**" title="Easily initialize environments and deploy Radius applications" >}}
  Radius codifies and automates the best practives for your hosting platform.

  We take care of things like connection strings, managed identities, role-based access control, and more.
{{< /card >}}

{{< /cardpane >}}
{{< cardpane >}}

{{< card header="**Easy application deployment & management**" title="Descriptive framework for cloud-native applications" >}}
  Radius environments and the rad CLI make it easy to test applications locally and deploy to production from developer macgines to CI/CD pipelines.

  Stop managing multiple test and deployment systems for your different pipelines.
{{< /card >}}

{{< card header="**Built-in Dapr support**" title="Easily incorporate Dapr building blocks into your applications" >}}
  Radius allows you to easily add Dapr sidecars and components into your applications and deploy them across cloud and edge.

  Radius + Dapr makes your apps completely portable across cloud + edge.
  
  [**Learn more about Dapr support**]({{< ref dapr-components >}})
{{< /card >}}

{{< /cardpane >}}

## Radius pieces

{{< cardpane >}}

{{< card header="**📃 Application model**" title="Descriptive framework for cloud-native applications" >}}
  Model your Radius Application using **Components**, **Scopes**, **Traits**, and **Deployments** which describe the functionality of your app.
  
  Radius uses the Bicep language as its file format and structure.
  
  [**Start modeling your applications**]({{< ref overview-concept.md >}})
{{< /card >}}

{{< card header="**⌨ rad CLI**" title="Easily initialize and deploy Radius applications" >}}
  The rad CLI is your primary interface with Radius environments and applications.

  Developers can initialize environments, deploy applications, view logs, check status, and more.
  
  [**Install the rad CLI**]({{< ref install-cli.md >}})
{{< /card >}}

{{< /cardpane >}}
{{< cardpane >}}

{{< card header="**☁ Managed environments**" title="Descriptive framework for cloud-native applications" >}}
  A Radius environment is where you can deploy and host Radius applications.
  
  It includes a **control-plane** which communicates with with the rad CLI and a **runtime** to which applications are deployed.
  
  [**Initialize Radius on your platform**]({{< ref environments.md >}})
{{< /card >}}

{{< card header="**🔌 Extendable components**" title="Deploy your applications to cloud and edge with zero code changes" >}}
  Radius components let you model your compute and data and deploy across different Radius managed environments with no changes to your code.

  No more platform-specific pipelines and bindings to swap out and maintain.
  
  [**Check out the Radius components**]({{< ref overview-concept.md >}})
{{< /card >}}


{{< /cardpane >}}