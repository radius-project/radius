for pod in $(kubectl get pods -n radius-system | awk 'NR>1{print $1}'); do
    echo "Namespace 'radius-system' is not empty"
    exit 1
  done
  exit 0

