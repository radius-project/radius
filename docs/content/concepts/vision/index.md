---
type: docs
title: Vision for Project Radius 
linkTitle: Vision
description: How Project Radius fits into the app development landscape and the long-term vision for its offerings. 
weight: 100
---

## Cloud-native apps are too difficult today 
App developers don't have a way to view and manage apps holistically. Instead, they have lists of disjoint resources and connection information that is passed around between them. Plus, the wide range of infrastructure types (cloud, on-premises, serverless) increases the management burden of keeping these resources connected and healthy.

What's missing is a way to bring the entire concept of an application into a single entity so it can be deployed, managed, and scaled easily.

## Project Radius empowers app developers to do more
<div class="-bg-light p-3" style="font-weight:bold;">
Build a unified concept of your “application”.
</div>

- Visualize your end-to-end app model. 
- Invetsigate cross-app health and diagnostics, including dependencies and connections. 
- Identify ownership and locate artifacts per component. 
- Support handoffs between teams as the app matures. 
- Easily layer IT policies across an app (access, backup, ...).

<br>
<div class="-bg-light p-3" style="font-weight:bold;">
Drastically reduce infra ops time.
</div>

- Iterate quickly in a local dev environment, then scale that same app up in Azure or Kubernetes.
- Stamp out versions of the app in multiple geos or clouds. 
- Follow best practices to be naturally secure by default, even with many teams working together. 


## Project Radius has a low barrier to entry
The Radius platform is comprised of a human-readable language for describing applications and a suite of supporting tools.   

#### For existing apps

Teams can easily represent their existing app resources in the Radius language, pulling together disparate pieces of their apps into a single view.   
Start using Radius to monitor cross-app health on Day 1. 

#### For new apps

Radius provides a hassle-free serverless experience. Applications defined with Radius are inherently portable across platforms. It takes minutes to define per-platform specs and move a local dev project to the cloud without rewriting the app. 

## Dive into nitty-gritty app details - if you want. 
Radius provides a flexibile model that meets developers where they need it to:  
- easy out-of-the-box defaults for simple v1 explorations
- ability to tune low-level settingsfully-fledged "pager-on-belt" applications 

## Applications as code

With the Radius app model, teams can easily codify and share pieces of an application. For example, a container with app code owned by one team can seamlessly connect to a database owned by a second team. 
{{< rad file="snippets/appmodel-concept.bicep" embed=true >}}

The result is no longer just a flat list of resources - it's a fully fledged diagram of how the pieces relate to each other.
{{< imgproc ui-mockup-basic Fit "700x500">}}
<i>An example app represented in the Radius Azure Service.</i>
{{< /imgproc >}}

In fact, we're committed to creating a dev experience users love. So developers will be able to debug and iterate on that same app definition locally via VSCode as well. 
<!-- TODO: make all these diagrams & code show the identically same app -->
{{< imgproc vscode-mockup-basic Fit "700x500">}}
<i>An example app represented in VSCode.</i>
{{< /imgproc >}}

## Platform strategy

Project Radius aims to support all hosting platform types - from hyperscale cloud, to self-hosted Kubernetes on the edge, to IoT and edge devices.

{{< imgproc platform-goals Fit "700x500" >}}
{{< /imgproc >}}

Our current focus is on delivering robust support for the following platforms:

- [Local development]({{< ref local >}}) as part of a developer inner-loop
- [Microsoft Azure]({{< ref azure>}}) as a managed-application serverless PaaS
- [Kubernetes]({{< ref kubernetes >}}) in all flavors and form-factors


<br>
{{< button text="Learn about the app model" page="appmodel-concept" >}}
