## Deploying and accessing Azure KeyVault using Radius

Let's look at the manual steps involved for an Azure Keyvault to be made accessible inside a containerized application:

1. Create AKS cluster with Managed Identity and Pod Identity Profile enabled
2. Grant at least "Managed Identity Operator" permissions to the AKS Cluster Identity to be able to associate pod identity with pod
3. Create a User Assigned Managed Identity
4. Grant Keyvault access to the User Assigned Managed Identity created
5. Create an AAD Pod Identity which creates a binding between a pod label and the User Assigned Managed Identity created above. You can use the commands below to check the pod identity and the binding created.

    ```sh
    kubectl get azureidentities.aadpodidentity.k8s.io -n <your namespace>
    kubectl describe azureidentities.aadpodidentity.k8s.io -n <your namespace>
    ```

    Note that the pod identity should be created in the same namespace as the application container.

6. Modify the k8s spec for the container to use Pod Identity label.

    ```sh
    apiVersion: v1
    kind: Pod
    metadata:
    name: demo
    labels:
        aadpodidbinding: POD_IDENTITY_NAME
    ```

7. Create application container in the same namespace as the Pod Identity namespace.

Radius automates all these steps and the user application can simply use the Azure KeyVault deployed by the spec.
