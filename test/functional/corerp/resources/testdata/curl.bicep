resource app 'radius.dev/Application@v1alpha3'= {
  name: 'curl'

  resource curlcontainer 'Container@v1alpha3' = {
    name:'curl'
    properties: {
      container: {
        image: 'tommyniu.azurecr.io/curl:latest'
      }
    }
  }
}
