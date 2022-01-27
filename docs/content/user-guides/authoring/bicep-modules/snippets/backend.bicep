param app object
param mongo object

resource myapp 'radius.dev/Application@v1alpha3' existing = {
  name: app.name

  resource backendHttp 'HttpRoute' = {
    name: 'backend-http'
    properties: {
      port: 80
    }
  }
  
  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'nginx:latest'
        ports: {
          web: {
            containerPort: 80
            provides: backendHttp.id
          }
        }
      }
      connections: {
        mongo: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
    }
  }

}

output backendHttp object = myapp::backendHttp
