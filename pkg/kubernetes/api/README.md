# Webhooks and Kubernetes Controllers

These contain meeting/presentation notes on webhooks and controllers with a radius kubernetes environment.

## Validating Webhooks

Problem: I have defined an application and I want to deploy to k8s. However, I poorly defined my radius file with something that will fail due to improper configuration.

ex: I configured `replicas` on the dapr trait, which is only valid for the manual scaling trait.

We added json schema and open api validation to the Radius RP. However we can levarage this as well to validate k8s CRDs.

### What is a Webhook?

Webhooks (also known as admission webhooks) are a way to validate and/or modify resources before they are created, updated, or deleted.

Two kinds of webhooks are supported:
- Mutating Webhooks: Invoked first and can modify the object being created
- Validating Webhooks: Can reject requests and enforce custom policies.

Today we leverage Validating Webhooks to validate k8s CRDs, but it seems like Mutating Webhooks are a superset of Validating Webhooks, so they are somewhat interchangeable for validating scenarios.

Our validating webhooks today verify that the CRD is valid according to the JSON schema.

### Declaritive validation

Truthfully, it's a bit odd that we are doing json schema validation in the validating webhooks. Normally, schema validation would be handled by the openAPIV3Schema section in a CRD (see [the component crd as an example](../../../deploy/Chart/crds/radius.dev_components.yaml)). Validating webhooks are more situated for scenarios that can't be validated by schema, like checking that a DNS name is well formed.

The reason we use validating webhooks for now is that the openAPIV3Schema doesn't support discriminated unions sufficiently (see [structural schema](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#specifying-a-structural-schema)). We spent a bit of time trying to figure out if we could splice it together, however we deemed it too difficult to do versus just validating in the webhook.

However, with the changes to AppModelV3, we should revisit this and see if we can leverage this to the fullest.

## Kubernetes Controllers

Problem: I have a deployment (`rad deploy foo.bicep`) that I want to deploy to kubernetes. Today, we have two controllers for processing applications and components that are deployed, but this means the applications and components are sent from the client to the server, rather than the server handling the entire deployment.

Downsides of client side model (and hence we would never have it long term):
- Would need clientside to implement ordering
- Would need to keep connection open between client and server until deployment is complete
- Doesn't match at all what we do in Azure (all server side)

Solution: We have a controller that handles the deployment of applications and components.

The deploymenttemplate controller and CRD contain the arm representation of the deployment. The controller is responsible for determining whether we need to create, update, or delete components/applications and applying them to the cluster.

### Two webhooks

We now do validation in two places for validating webhooks (validation on the overall deployment as well as individual components/applications).

### Long term

After adding this deploymenttemplate controller, we can add the following features:
- Ordering deployments
- Tracking deployments based off readiness (`rad deployments show` can show the progress of a deployment or `rad deploy` will wait until the deployment is actually complete)

### Long Long term

The work we are doing here will _eventually_ be somewhat duplicated by the open source arm deployment engine.
