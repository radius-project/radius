---
type: docs
title: Overview of Project Radius vision
linkTitle: Vision
description: An intro to how Project Radius fits into the app development landscape and the long-term vision for its offerings. 
weight: 100
---

## Deploying and managing cloud-native apps is too hard today. 

Neither Azure nor Kubernetes have a way to view and manage apps holistically.     The wide range of infrastructure types (cloud, on-premises, serverless) increases the management burden.

<br>
<div class="row ">
<div class="col-lg-4 mb-5 mb-lg-0 text-left" style="background-color:#FFA630;border-width: 6px; border-color: white;border-style:solid;text-align: center;">
    <h4 class="h4" style="margin: 10px;">
    Developers don’t have a common concept of an “application”.
    </h4>
</div>

<div class="col-lg-4 mb-5 mb-lg-0 text-left" style="background-color:#FFA630;border-width: 6px; border-color: white;border-style:solid;">
    <h4 class="h4" style="margin: 10px;"> 
        Developers need to be infra ops specialists
    </h4>
</div>	
</div>


## Mission statement

{{% alert color="primary" %}}
An intelligent application model that empowers developers to easily deploy and manage applications with a serverless experience.
{{% /alert %}}

Radius aims to be:
- A community-developed open-source project
- The standard first-class managed application concept in Azure
- Loved by developers building applications

## Radius enables users to: 

{{< cardpane >}}
{{< card header="**Have a single app-level concept**" >}}
- Visualize end-to-end app model. 
- Invetsigate cross-app health and diagnostics, including dependencies and connections. 
- Understand how resource configuration changes affect the app. 
- _Adjust components while maintaining the health of the whole app. ....change this one??_
{{< /card >}}
{{< card header="**Build portable apps**" >}}
- Iterate quickly in a local dev environment, then scale that same app up in Azure or Kubernetes.
- Stamp out versions of the app in multiple geos or clouds. 
- _Something else..._
{{< /card >}}
{{< /cardpane >}}
{{< cardpane >}}
{{< card header="**Be more productive**" >}}
- See how the app is put together. 
- Bootstrap robust CI/CD processes. 
- Rapid inner loop dev. 
- _Intelligently automate "wiring up" work that devs don't need to learn about. ....change this one??_
{{< /card >}}
{{< card header="**Cultivate their apps**" >}}
- Identify ownership and locate artifacts per component. 
- Support handoffs between teams as the app matures. 
- Easily layer IT policies on the app (access, backup, ...).
- Follow best practices to be naturally secure by default, even with many teams working together. 
{{< /card >}}
{{< /cardpane >}}



## Platform strategy

Radius will support all hosting platforms - from major public clouds, to Kubernetes on Raspberry Pi, to IoT and edge devices. 

Our current focus is on delivering robust support for the following platforms:

- [Local development]({{< ref local >}}) as part of a developer inner-loop
- [Microsoft Azure]({{< ref azure>}}) as a managed-application serverless PaaS
- [Kubernetes]({{< ref kubernetes >}}) in all flavors and form-factors

<br>
{{< button text="Learn about the app model" page="appmodel-concept" >}}