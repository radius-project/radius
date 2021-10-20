param app object

resource seq 'radius.dev/Application/ContainerComponent@v1alpha3' = {
  name: '${app.name}/seq'
  properties: {
    container: {
      image: 'datalust/seq:latest'
      env: {
        'ACCEPT_EULA': 'Y'
      }
      ports: {
        web: {
          containerPort: 80
          provides: seqHttp.id
        }
      }
    }
    traits: []
    connections: {}
  }
}

resource seqHttp 'radius.dev/Application/HttpRoute@v1alpha3' = {
  name: '${app.name}/seq-http'
  properties: {
    port: 5340
  }
}

output seq object = seq
output seqHttp object = seqHttp
