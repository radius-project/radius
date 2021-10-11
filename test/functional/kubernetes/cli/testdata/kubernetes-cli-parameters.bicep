param registry string
param env string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-cli-parameters'

  resource a 'ContainerComponent' = {
    name: 'a'
    properties: {
      container: {
        image: '${registry}/magpie:latest'
        env: {
          COOL_SETTING: env
        }
      }
    }
  }

  resource b 'ContainerComponent' = {
    name: 'b'
    properties: {
      container: {
        image: '${registry}/magpie:latest'
      }
    }
  }
}
