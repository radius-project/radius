helm uninstall gitea --namespace gitea
kubectl delete ns gitea

helm repo add gitea-charts https://dl.gitea.io/charts/
helm repo update
helm install gitea gitea-charts/gitea --namespace gitea --create-namespace -f /Users/willsmith/dev/radius/radius/.github/actions/install-gitea/gitea-config.yaml
kubectl wait --for=condition=available deployment/gitea -n gitea --timeout=120s

gitea_pod=$(kubectl get pods -n gitea -l app=gitea -o jsonpath='{.items[0].metadata.name}')
output=$(kubectl exec -it $gitea_pod -n gitea -- gitea admin user create --admin --username testuser --email testuser@radapp.io --password giteaadmin --must-change-password=false)
echo $output
output=$(kubectl exec -it $gitea_pod -n gitea -- gitea admin user generate-access-token --username testuser --token-name radius-functional-test-gitea-access-token  --scopes "write:repository,write:user" --raw)
echo $output