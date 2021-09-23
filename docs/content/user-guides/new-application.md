---
type: docs
title: "Create a new application with Project Radius"
linkTitle: "Create new apps"
description: "Learn how to use Radius for your new applications"
weight: 200
---

Project Radius is the easiest way to create a new application across a [variety of platforms]({{< ref platforms >}}). With managed resources, Radius can manage the lifecycle of your infrastructure in addition to your services.

## Model your services in Radius

Begin my modeling your services in Radius components, such as `ContainerComponent` resources. In this example we're modeling a simple website:

```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource website 'ContainerComponent@v1alpha3' = {
    name: 'website'
    properties: {
      container: {
        image: 'nginx:latest'
      }
    }
  }
}
```

### Add Radius components

Now that your service is modeled, you can add [additional components]({{< ref components-model >}}) that represent your databases, message queues, and other application resources. In this example we're adding a MongoDB:

```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource mongodb 'mongo.com.mongodb' = {
    name: 'mongodb'
    properties: {
      config: {
        managed: true
      }
    }
  }

  resource website 'ContainerComponent@v1alpha3' = {
    name: 'website'
    properties: {
      container: {
        image: 'nginx:latest'
      }
      connections: {
        datastore: {
          resource: mongodb.id
        }
      }
    }
  }
}
```

{{% alert title="Managed resources" color="primary" %}}
Notice that `managed:true` is set, meaning Radius is responsible for managing the lifecycle of the resource. If you manage your resources outside of your Radius application, you can use the `resource` property to specify a resource id. See the [existing applications guide]({{< ref existing-application.md >}}) for an example.
{{% /alert %}}

You can now deploy your application to [any Radius platform]({{< ref platforms >}}), and a MongoDB will be created for you and made available to `website`.

## Next steps

- Visit the [Components section]({{< ref components-model >}}) to begin using Radius components in your application.
- Try out a [tutorial]({{< ref tutorial >}}) to learn what capabilities Radius adds to your application.