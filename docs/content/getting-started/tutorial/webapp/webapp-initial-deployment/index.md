---
type: docs
title: "Deploy the website tutorial frontend"
linkTitle: "Deploy frontend"
description: "Deploy the website tutorial frontend in a container"
weight: 2000
---


## Define a Radius app as a .bicep file

Radius uses the [Bicep language](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/bicep-overview) as its file-format and structure. In this tutorial you will define an app named `webapp` that will contain the container and database components, all described in Bicep.

Create a new file named `template.bicep` and paste the following:

```sh
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

}
```

## Add a container component 

Next you'll add a `todoapp` component for the website's frontend.

Radius captures the relationships and intentions behind an application, which simplifies deployment and management. The single `todoapp` component in your template.bicep file will contain everything needed for the website frontend to run. 

Your `todoapp` component will specify:  
- **kind:** `radius.dev/Container@v1alpha1`, a generic container. 
- **container image:** `radiusteam/tutorial-todoapp`, a Docker image the container will run. This is where your website's front end code lives. 
- **bindings:** `http`, a Radius binding that adds the ability to listen for HTTP traffic (on port 3000 here).




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
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
    }
  }
}
```

Note that you don't have to interact with multiple Resource Providers or manage details like connection string injection.   

## Deploy the application 

Now you are ready to deploy the application for the first time. 

> **Reminder:** At this point, you should already be logged into the az CLI and already have an [environment initialized]({{< ref create-environment.md >}}). 

1. Deploy to your Radius environment via the rad CLI:

   ```sh
   rad deploy template.bicep
   ```

   This will deploy the application into your environment and launch the container resource for the frontend website. 

1. Confirm that your Radius application was deployed:

   ```sh
   rad component list --application webapp
   ```
   
   You should see your `todoapp` component. Example output: 
   ```
   COMPONENT  KIND
   todoapp    radius.dev/Container@v1alpha1 
   ```

1. To test your `webapp` application, open a local tunnel to your application:

   ```sh
   rad component expose todoapp --application webapp --port 3000
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

<br>{{< button text="Next: Add a database to the app" page="webapp-add-database.md" >}}
