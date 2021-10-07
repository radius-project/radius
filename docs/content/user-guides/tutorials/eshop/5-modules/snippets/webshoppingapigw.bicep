param app object

// Based on https://github.com/dotnet-architecture/eShopOnContainers/tree/dev/deploy/k8s/helm/apigwws
resource webshoppingapigw 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/webshoppingapigw'
  properties: {
    container: {
      image: 'envoyproxy/envoy:v1.11.1'
      env: {}
      ports: {
        http: {
          containerPort: 80
          provides: webshoppingapigwHttp.id
        }
        http2: {
          containerPort: 8001
          provides: webshoppingapigwHttp2.id
        }
      }
    }
    traits: []
    connections: {}
  }
}

resource webshoppingapigwHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webshoppingapigw-http'
  properties: {
    port: 5202
  }
}

resource webshoppingapigwHttp2 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/webshoppingapigw-http-2'
  properties: {
    port: 15202
  }
}

output webshoppingapigw object = webshoppingapigw
output webshoppingapigwHttp object = webshoppingapigwHttp
output webshoppingapigwHttp2 object = webshoppingapigwHttp2
