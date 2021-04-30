---
type: docs
title: "Deploy a basic website app"
linkTitle: "2. Deploy the website"
description: "Deploy the website frontend in a container"
weight: 50
---


## Define a Radius app as a .bicep file

Radius uses the [Bicep](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/bicep-overview) langauge as its file-format and structure. Create a new file named `template.bicep`.

In this tutorial, you'll define an app named `webapp` that will eventually contain components. 

```
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

}
```

## Add a container component 

Next you'll add a `todoapp` component for the website's frontend.

Radius captures the relationships and intentions behind an application, which simplifies deployment and management. The single `todoapp` component in your template.bicep file will contain everything needed for the website frontend to run. 

Your `todoapp` component will specify:  
- **kind:** `radius.dev/Container@v1alpha1`, which represents a generic container. 
- **container image:** `radiusteam/tutorial-todoapp`, which says which Docker image the countainer will run. This is where your website's front end code lives. 
- **provides:** `http`, which is a Radius service that adds the ability to listen for HTTP traffic (on port 3000 here).

Update your template.bicep file to match the full application definition:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
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

Note that you don't have to interact with multiple Resource Providers or manage details like Connection String injection.   

## Deploy the application 

Now you are ready to deploy the application for the first time. 
Reminder: At this point, you should already be logged into the az CLI and already have an environment initialized. 

1. Run:

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application into your environment and launch the container resource for the frontend website. 

1. Confirm that your Radius application was deployed:

   ```sh
   rad application list
   ```
   
   You should see your `webapp` application. Example output: 
   ```
   {
     "value": [
       {
         "id": "/subscriptions/{SUB-ID}/resourceGroups/{RESOURCE-GROUP}/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/webapp",
         "name": "webapp",
         "type": "Microsoft.CustomProviders/resourceProviders/Applications"
       }
     ]
   }
   ```

1. To test your `webapp` application, open a local tunnel to your application:

   ```sh
   rad expose webapp todoapp --port 3000
   ```

   {{% alert title="💡 rad expose" color="primary" %}}
   The `rad expose` command provides the application name, followed by the component name, followed by a port. If you changed any of these names when deploying, update your command to match.
   {{% /alert %}}

1. Visit the URL [http://localhost:3000](http://localhost:3000) in your browser. For now you should see a page like:

   <img src="todoapp-nodb.png" width="400" alt="screenshot of the todo application with no database">

   If the page you see matches the screenshot, that means the container is running as expected. 

   You can play around with the application's features features:
   - Add a todo item
   - Mark a todo item as complete
   - Delete a todo item

1. When you're done testing press CTRL+C to terminate the port-forward.

<br /><a class="btn btn-primary" href="{{< ref add-database.md >}}" role="button">Next: Add a database to the app</a>
