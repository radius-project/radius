resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-container-httpbinding'
  // resource frontendhttp 'HttpRoute' = {
  //   name: 'frontendhttp'
  //   properties: {
  //     port: 80
  //   }
  // }
  // resource frontend 'ContainerComponent' = {
  //   name: 'frontend'
  //   properties: {
  //     connections: {
  //       backend: {
  //         kind: 'Http'
  //         source: backendhttp.id
  //       }
  //     }
  //     container: {
  //       image: 'radius.azurecr.io/magpie:latest'
  //       ports: {
  //         web: {
  //           containerPort: 80
  //           provides: frontendhttp.id
  //         }
  //       }
  //       env: {
  //         SERVICE__BACKEND__HOST: backendhttp.properties.host
  //         SERVICE__BACKEND__PORT: '${backendhttp.properties.port}'
  //       }
  //     }
  //     traits: [
  //       {
  //         kind: 'radius.dev/InboundRoute@v1alpha1'
  //         binding: 'web'
  //       }
  //     ]
  //   }
  // }
  // resource backendhttp 'HttpRoute' = {
  //   name: 'backendhttp'
  // }
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        // ports: {
        //   web: {
        //     containerPort: 80
        //     provides: backendhttp.id
        //   }
        // }
      }
    }
  }
}
