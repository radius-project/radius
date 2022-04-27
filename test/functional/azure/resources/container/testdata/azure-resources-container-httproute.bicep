resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-httproute'

  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      connections: {
        backend: {
          kind: 'Http'
          source: backendhttp.id
        }
      }
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        env: {
          SERVICE__BACKEND__HOST: backendhttp.properties.host
          SERVICE__BACKEND__PORT: string(backendhttp.properties.port)
        }
      }
    }
  }

  resource backendhttp 'HttpRoute' = {
    name: 'backendhttp'
  }

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'rynowak/backend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: backendhttp.id
          }
        }
        volumes:{
          'my-volume':{
            kind: 'ephemeral'
            mountPath:'/tmpfs'
            managedStore:'memory'
          }
        }
      }
    }
  }
}
