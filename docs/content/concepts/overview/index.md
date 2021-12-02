---
type: docs
title: Vision for Project Radius 
linkTitle: Vision
description: How Project Radius fits into the app development landscape and the long-term vision for its offerings. 
weight: 100
---

## Cloud-native apps are too difficult today. 
App developers don't have a way to view and manage apps holistically.  Plus, the wide range of infrastructure types (cloud, on-premises, serverless) increases the management burden. 

We need a way to wrangle the entire concept of an app into a single picture so it can be managed, moved around, and scaled easily. 

## Radius empowers app developers to do more. 
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

- Use built-in "wiring up" functionality to implement connections. 
- Iterate quickly in a local dev environment, then scale that same app up in Azure or Kubernetes.
- Stamp out versions of the app in multiple geos or clouds. 
- Follow best practices to be naturally secure by default, even with many teams working together. 


## Radius has a low barrier to entry. 
The Radius platform is comprised of a human-readable language for describing applications and a suite of supporting tools.   

#### For existing apps - 
Teams can easily represent their existing app resources in the Radius language, pulling together disparate pieces of their apps into a single view.   
Start using Radius to monitor cross-app health on Day 1. 

#### For new apps - 
Radius provides a hassle-free serverless experience. Applications defined with Radius are inherently portable across platforms. It takes minutes to define per-platform specs and move a local dev project to the cloud without rewriting the app. 

## Dive into nitty-gritty app details - if you want. 
Radius provides a flexibile model that meets developers where they need it to:  
- easy out-of-the-box defaults for simple v1 explorations
- ability to tune low-level settingsfully-fledged "pager-on-belt" applications 

## Platform strategy

Eventually, Radius will support all hosting platforms - from major public clouds, to Kubernetes on Raspberry Pi, to IoT and edge devices. 

{{< imgproc platform-goals Fit "700x500" >}}
{{< /imgproc >}}

Our current focus is on delivering robust support for the following platforms:

- [Local development]({{< ref local >}}) as part of a developer inner-loop
- [Microsoft Azure]({{< ref azure>}}) as a managed-application serverless PaaS
- [Kubernetes]({{< ref kubernetes >}}) in all flavors and form-factors


<br>
{{< button text="Learn about the app model" page="appmodel-concept" >}}
