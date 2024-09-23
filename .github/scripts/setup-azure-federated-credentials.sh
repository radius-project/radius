if [ "$#" -ne 4 ]; then
    echo "Usage: $0 <K8S_CLUSTER_NAME> <AZURE_RESOURCE_GROUP> <AZURE_SUBSCRIPTION_ID> <OIDC_ISSUER_URL>"
    exit 1
fi

export K8S_CLUSTER_NAME=$1
export AZURE_RESOURCE_GROUP=$2
export AZURE_SUBSCRIPTION_ID=$3
export SERVICE_ACCOUNT_ISSUER=$4

# Create the Entra ID Application
export APPLICATION_NAME="${K8S_CLUSTER_NAME}-radius-app"
az ad app create --display-name "${APPLICATION_NAME}"

# Get the client ID and object ID of the application
export APPLICATION_CLIENT_ID="$(az ad app list --display-name "${APPLICATION_NAME}" --query [].appId -o tsv)"
export APPLICATION_OBJECT_ID="$(az ad app show --id "${APPLICATION_CLIENT_ID}" --query id -otsv)"

# Create the applications-rp federated credential for the application
cat <<EOF > params-applications-rp.json
{
  "name": "radius-applications-rp",
  "issuer": "${SERVICE_ACCOUNT_ISSUER}",
  "subject": "system:serviceaccount:radius-system:applications-rp",
  "description": "Kubernetes service account federated credential for applications-rp",
  "audiences": [
    "api://AzureADTokenExchange"
  ]
}
EOF
az ad app federated-credential create --id "${APPLICATION_OBJECT_ID}" --parameters @params-applications-rp.json

# Create the bicep-de federated credential for the application
cat <<EOF > params-bicep-de.json
{
  "name": "radius-bicep-de",
  "issuer": "${SERVICE_ACCOUNT_ISSUER}",
  "subject": "system:serviceaccount:radius-system:bicep-de",
  "description": "Kubernetes service account federated credential for bicep-de",
  "audiences": [
    "api://AzureADTokenExchange"
  ]
}
EOF
az ad app federated-credential create --id "${APPLICATION_OBJECT_ID}" --parameters @params-bicep-de.json

# Create the ucp federated credential for the application
cat <<EOF > params-ucp.json
{
  "name": "radius-ucp",
  "issuer": "${SERVICE_ACCOUNT_ISSUER}",
  "subject": "system:serviceaccount:radius-system:ucp",
  "description": "Kubernetes service account federated credential for ucp",
  "audiences": [
    "api://AzureADTokenExchange"
  ]
}
EOF
az ad app federated-credential create --id "${APPLICATION_OBJECT_ID}" --parameters @params-ucp.json

echo "------- app id -------"
echo ${APPLICATION_CLIENT_ID}
# Set the permissions for the application
az ad sp create --id ${APPLICATION_CLIENT_ID}
az role assignment create --assignee "${APPLICATION_CLIENT_ID}" --role "Owner" --scope "/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${AZURE_RESOURCE_GROUP}"