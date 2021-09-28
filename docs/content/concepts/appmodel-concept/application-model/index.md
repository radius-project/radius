---
type: docs
title: "Radius applications"
linkTitle: "Applications"
description: "Learn how to model your applications with the Radius application."
weight: 100
---

## Definition

The Radius Application contains everything on the diagram, including all the compute, data, and infrastructure. In one line, the Application is defined as: 

{{% alert title="📄 Application" color="primary" %}}
The biggest possible circle you could draw around your software.
{{% /alert %}}

In Radius, the Application concept is the boundary within which names have meaning. The other concepts in Radius define symbolic names - the Application is the scope in which these names must be unique, and in which they are referenced.

{{< imgproc radius-application Fit "700x500">}}
<i>A Radius Application encompases all the containers, databases, and APIs within a simple online store app.</i>
{{< /imgproc >}}

### Bicep example

This example shows a blank application with nothing in it. Deploying this would do nothing as there are no [components]({{< ref components-model.md >}}).

{{< rad file="snippets/blank-app.bicep" embed=true >}}

{{% alert title="💡 Key concept" color="info" %}}
Since defining an Application only defines a boundary, deploying an empty Application has no real effect. Applications are only containers for your Components.
{{% /alert %}} 

It's up to you, the user, to define what you consider part of the Application. It is recommended that you draw this circle very large to includes as much as possible of the software you work on. You should include the things that run your code (*runnable components*), and your data and infrastructure resources (*non-runnable components*).

{{% alert title="💡 Key concept" color="info" %}}
Applications are **not** units of deployment. An Application can contain multiple units of deployment that version separately. This topic will be explored later.
{{% /alert %}} 

## Next step

Now that you are familiar with Radius applications, the next step is to learn about Radius components.

{{< button text="Learn about Components" page="components-model.md" >}}
