param registry string
param env string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-cli-parameters'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: '${registry}/magpiego:latest'
        env: {
          COOL_SETTING: env
        }
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: '${registry}/magpiego:latest'
      }
    }
  }
}
