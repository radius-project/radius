---
type: docs
title: "Add a content generator to the app"
linkTitle: "Add a content generator"
description: "How to add a content generator to the tutorial application"
weight: 4000
---


To complete the application, you'll add another component for the order generating microservice. 

Again, we'll discuss changes to template.bicep and then provide the full, updated file before deployment.

## Add pythonapp component
Another container component is used to specify a few properties about the order generator: 

- **kind:** `radius.dev/Container@v1alpha1`, a generic container. 
- **container image:** `radiusteam/tutorial-pythonapp`, a Docker image the container will run.
- **uses:** `nodeapp`, which declares the intention for `pythonapp` to communicate with `nodeapp` using `dapr.io/Invoke` as the protocol. 
- **traits:** `appId: pythonapp`, required Dapr configuration. 

```sh
  resource pythonapplication 'Components' = {
    name: 'pythonapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-pythonapp'
        }
      }
      uses: [
        {
          binding: nodeapp.properties.bindings.invoke
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'pythonapp'
        }
      ]
    }
  }
```

A few notable differences between `pythonapp` and the previously-deployed `nodeapp`:

- `pythonapp` doesn't listen for HTTP traffic, so it configures neither a Dapr app-port nor a binding for HTTP.
- `pythonapp` needs to communicate with `nodeapp` using the Dapr service invocation protocol.

## Update your template.bicep file 

Update your `template.bicep` file to match the full application definition:

{{%expand "❗️ Expand for the full code block" %}}
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
      uses: [
        {
          binding: statestore.properties.bindings.default
        }
      ]
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
        invoke: {
          kind: 'dapr.io/Invoke'
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'nodeapp'
          appPort: 3000
        }
      ]
    }
  }

  resource pythonapplication 'Components' = {
    name: 'pythonapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-pythonapp'
        }
      }
     uses: [
        {
          binding: nodeapp.properties.bindings.invoke
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'pythonapp'
        }
      ]
    }
  }

  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'state.azure.tablestorage'
        managed: true
      }
    }
  }
}
```
{{% /expand%}}  
  
## Deploy application with pythonapp

1. Switch to the command line and run:

   ```sh
   rad deploy template.bicep
   ```

1. To test out the pythonapp microservice, open a local tunnel on port 3000 once more:

   ```sh
   rad component expose nodeapp --application dapr-hello --port 3000
   ```

1. Visit [http://localhost:3000/order](http://localhost:3000/order) in your browser.

   Refresh the page multiple times and you should see a message like before. The order number is steadily increasing after refresh (incremented by `pythonapp`). For example:

   `{"orderId":27}`

   If your message is similar, then `pythonapp` is able to communicate with `nodeapp`. 

1. When you're done testing press CTRL+C to terminate the port-forward. 

## Next steps

- If you'd like to try another tutorial with your existing environment, go back to the [Radius tutorials]({{< ref tutorial >}}) page. 
- If you're done with testing, use the rad CLI to [delete an environment]({{< ref rad_env_delete.md >}}) to **prevent additional charges in your subscription**. 
- Related links for Dapr:
  - [Dapr documentation](https://docs.dapr.io/)
  - [Dapr quickstarts](https://github.com/dapr/quickstarts/tree/v1.0.0/hello-world)

You have completed this tutorial!

<br>{{< button text="Try another tutorial" page="tutorial" >}}