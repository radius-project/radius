resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'my-application'
}

module frontend 'br:radius.azurecr.io/templates/container:latest' = {
  name: 'frontend-module'
  params: {
    app: myapp
    name: 'frontend'
    image: 'radius.azurecr.io/services/frontend:latest'
    ports: {
      web: {
        containerPort: 80
      }
    }
    connections: {
    }
  }
}

module backend 'br:radius.azurecr.io/templates/container:latest' = {
  name: 'backend-module'
  params: {
    app: myapp
    name: 'backend'
    image: 'exampleregistry.azurecr.io/services/backend:latest'
    ports: {
      web: {
        containerPort: 80
      }
    }
    livenessPort: 3001
    connections: {
    }
  }
}
