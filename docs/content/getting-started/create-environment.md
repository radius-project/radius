---
type: docs
title: "Create a Radius RP environment"
linkTitle: "Create environment"
description: "How to initialize the private Radius resource provider in your Azure subscription"
weight: 30
---

Radius deploys a private resource provider, or control plane, that your `rad` cli connects to when managing your radius applications.

## Deploy a Radius environment

As a one time operation deploy an [Azure Radius environment]({{< ref azure-environments >}}) into your Azure subscription through the following instructions.

{{% alert title="⚠ Caution" color="warning" %}}
While Radius environments are optimized for cost, any costs incurred by the deployment and use of a Radius environment in an Azure subscription are the responsibility of the user. Learn more about Azure environments [here]({{< ref azure-environments >}}).
{{% /alert %}}

1. Use the `az` CLI to authenticate with Azure your Azure account:

   ```sh
   az login
   ```

1. Select your Azure subscription:

   Radius will use your default Azure subscription. You can verify your enabled subscription with:

   ```sh
   az account show
   ```

   If needed, you can switch your to your preferred subscription:

   ```sh
   az account set --subscription <SUB-ID>
   ```

1. Create a Radius environment:

   Initialize the private resource provider (environment) in your Azure subscription using the `rad` CLI. The following command creates an environment in interactive mode and will prompt you for input like resource-group name and location. 

   ```sh
   rad env init azure -i
   ```

   This will prompt you for several inputs and then go create assets in your subscription (~5-10 mins). 

   For more info about what's being created as part of an environment, see [environments]({{< ref environments >}}).

1. Verify creation of your new environment:

   ```sh
   rad env list
   ```

### Delete an environment

The rad CLI also has an option to [delete an environment]({{< ref rad_env_delete.md >}}) if you need to remove or re-deploy an environment.

## Next steps

Now that you have a Radius environment up and running head over to our tutorials section to walk through some applications and scenarios:

<a class="btn btn-primary" href="{{< ref tutorial >}}" role="button">Next: Try a tutorial</a>
