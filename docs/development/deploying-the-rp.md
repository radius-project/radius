# Deploying the RP

*This guide will cover deploying a Radius environment to Azure manually. This is part of the job that's done by `rad env init azure`. This worklow is useful when you need to test changes to the templates, or when you need to override a setting that's not provided by the CLI.* 

### Step 1: (Optional) Build & Push the Image

You'll need a place to push Docker images that is publicly accessible. Alternatively you can use the public images from `radiusteam/radius-rp`.

```sh
docker build . -t <your registry>/radius-rp:latest
docker push <your registry>/radius-rp
```

### Step 2: Create a Service Principal

```sh
az ad sp create-for-rbac --sdk-auth --role Contributor > "creds.json"
```

### Step 2: Deploy the ARM Template

**Create a resource group (if desired):**

```sh
az group create --name <resource group> --location <location>
```

**Deploy**

```sh
az deployment group create \
  --template-file deploy/rp-full.json \
  --resource-group <resource group> \
  --parameter "image=<repository>/radius-rp:latest" \
  --parameter "servicePrincipalClientId=$(jq '.clientId' --raw-output creds.json)" \
  --parameter "servicePrincipalClientSecret=$(jq '.clientSecret' --raw-output creds.json)" \
  --parameter "sshRSAPublicKey=$(<~/.ssh/id_rsa.pub)"
```

If you open the Azure portal and navigate to your resource group you should see an:
- App Service Plan
- App Service Site
- CosmosDB account
- Custom Resource Provider registration 
- Kubernetes Cluster
- Deployment Script
- Managed Identity
