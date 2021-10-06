---
type: docs
title: "Create a Radius  environment"
linkTitle: "Create environment"
description: "How to initialize a Radius environment in your Azure subscription or Kubernetes cluster"
weight: 30
---

## Deploy a Radius environment

{{< tabs "Azure" "Kubernetes" >}}

{{% codetab %}}

Note that while the custom resource provider and container runtime are optimized for cost, you are responsible for any costs incurred in your subscription.

1. Use the `az` CLI to authenticate with Azure your Azure account:

   ```sh
   az login
   ```

   {{% alert title="RBAC requirement" color="warning" %}}
   Radius environments on Azure currently require you to have *Owner* rights on your subscription. If you use a service principal for CLI authentication, ensure it also has *Owner* rights. This requirement is a temporary limitation.
   {{% /alert %}}

1. Create a Radius environment:

   The following command creates an environment in interactive mode and will prompt you for input like the name of a new Resource Group and location. If no environment name is specified, it will default to the Resource Group name.

   ```sh
   rad env init azure -i
   ```

   This will prompt you for several inputs and then go create assets in your subscription (~5-10 mins). 

   For more info about what's being created as part of an environment, see [Azure environments]({{< ref azure>}}).

1. Verify creation of your new environment:

   ```sh
   rad env list
   ```

{{% /codetab %}}

{{% codetab %}}
1. Verify that you have a Kubernetes cluster configured with a local `kubectl` context set as the default.
   To verify this, run `kubectl config current-context` and verify that it returns the name of your cluster.

1. Create a Radius environment:

   The following command creates an environment in the default Kubernetes namespace:

   ```sh
   rad env init kubernetes
   ```

   For more info about what's being created as part of an environment, see [Kubernetes environments]({{< ref kubernetes >}}).

1. Verify creation of your new environment:

   ```sh
   rad env list
   ```
{{% /codetab %}}

{{< /tabs >}}

## Delete an environment

The rad CLI also has an option to [delete an environment]({{< ref rad_env_delete.md >}}) if you need to remove or re-deploy an environment.

## Next steps

Now that you have a Radius environment up and running head over to our tutorials section to walk through some applications and scenarios:

{{< button text="Next: Try a tutorial" page="tutorials" >}}
