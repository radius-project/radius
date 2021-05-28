---
type: docs
title: "Radius applications"
linkTitle: "Applications"
description: "Learn how to model your applications in Radius with the Radius application."
weight: 100
---

Radius applications are the largest circle you could draw around your software. It is the boundary within which names have meaning. The other concepts in Radius define symbolic names - the Application is the scope in which these names must be unique, and in which they are referenced.

Radius applications are meaningless without the [components]({{< ref components-model.md >}}) that make them up. Components are the software and resources that make up an application.

## Example

This example shows a blank application with nothing in it. Deploying this would do nothing as there are no [components]({{< ref components-model.md >}}).

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'shopping-app'
}
```

## Next step

Now that you are familiar with Radius applications, the next step is to learn about Radius components.

{{< button text="Learn about components" page="components-model.md" >}}