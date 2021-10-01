resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //BACKEND
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'registry/container:tag'
        ports: {
          grpc: {
            containerPort: 3000
            provides: grpc.id
          }
        }
      }
    }
  }
  //BACKEND

  //ROUTE
  resource grpc 'GrpcRoute' = {
    name: 'grpcroute'
    properties: {
      port: 80
      gateway: {
        hostname: '*'
      }
    }
  }
  //ROUTE

  //FRONTEND
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env: {
          BACKEND_URL: grpc.properties.url
        }
      }
      connections: {
        grpc: {
          kind: 'Grpc'
          source: grpc.id
        }
      }
    }
  }
  //FRONTEND
  
}
