---
type: docs
title: "Deploy the Dapr microservices tutorial frontend"
linkTitle: "Deploy frontend"
description: "Deploy the application frontend in a container"
weight: 2000
---


## Define a Radius app as a .bicep file

Radius uses the [Bicep language](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/bicep-overview) as its file-format and structure. In this tutorial you will define an app named `dapr-hello` that will contain the container, statestore, and content generator components - all described in Bicep.

Create a new file named `template.bicep` and paste the following:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

}
```

## Add a container component 

Next you'll add a `nodeapp` component for the website's frontend.

Radius captures the relationships and intentions behind an application, which simplifies deployment and management. The single `nodeapp` component in your template.bicep file will contain everything needed for the website frontend to run. 

Your `nodeapp` component will specify:  
- **kind:** `radius.dev/Container@v1alpha1`, a generic container. 
- **container image:** `radiusteam/tutorial-nodeapp`, a Docker image the container will run. This is where your application's front end code lives. 
- **provides:** `http`, a Radius service that adds the ability to listen for HTTP traffic (on port 3000 here).


Update your template.bicep file to match the full application definition:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  resource nodeapplication 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-nodeapp'
        }
      }
      provides: [
        {
          kind: 'http'
          name: 'web'
          containerPort: 3000
        }
      ]
    }
  }
}
```

Note that you don't have to interact with multiple Resource Providers or manage details like connection string injection.   

## Deploy the application 

Now you are ready to deploy the application for the first time. 

> **Reminder:** At this point, you should already be logged into the az CLI and already have an environment initialized. 

1. Deploy to your Radius environment via the rad CLI:

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application into your environment and launch the container resource for the frontend website. 

1. Confirm that your Radius application was deployed:

   ```sh
   rad application list
   ```

   You should see your `dapr-hello` application. Example output: 
   ```
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/dapr-hello",
         "name": "radius/dapr-hello",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications"
       }
     ]
   }
   ```

1. To test your `dapr-hello` application, open a local tunnel to your application:

   ```sh
   rad expose dapr-hello nodeapp --port 3000
   ```

   {{% alert title="💡 rad expose" color="primary" %}}
   The `rad expose` command provides the application name, followed by the component name, followed by a port. If you changed any of these names when deploying, update your command to match.
   {{% /alert %}}

1. Visit the URL [http://localhost:3000/order](http://localhost:3000/order) in your browser. For now you should see a message like:

   `{"message":"The container is running, but Dapr has not been configured."}`

   If the message matches, then it means that the container is running as expected.

1. When you are done testing press `CTRL+C` to terminate the port-forward. 


<br>{{< button text="Next: Add a Dapr statestore to the app" page="dapr-microservices-add-dapr.md" >}}
