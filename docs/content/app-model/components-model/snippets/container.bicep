resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'storefront-app'

  //SAMPLE
  resource store 'Components' = {
    name: 'storefront'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //RUN
      run: { 
        container: {
          image: 'foo'
        }
      }
      //RUN
      //BINDINGS
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
      //BINDINGS
    }
  }
  //SAMPLE
}
