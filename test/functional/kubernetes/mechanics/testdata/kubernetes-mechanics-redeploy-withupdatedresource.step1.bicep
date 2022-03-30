param  magpieimage string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-mechanics-redeploy-withupdatedresource'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }
}
