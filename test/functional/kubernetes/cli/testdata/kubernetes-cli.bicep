param  magpieimage string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-cli'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }
}
