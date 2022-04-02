param registry string
param env string
param magpietag string = 'latest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-cli-parameters'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: '${registry}/magpiego:${magpietag}'
        env: {
          COOL_SETTING: env
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: '${registry}/magpiego:${magpietag}'
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
}
