resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'storefront-app'

  //SAMPLE
  resource store 'Components' = {
    name: 'storefront'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: { 
        container: {
          image: 'foo'
        }
      }
      //HIDE
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
    }
  }
  //SAMPLE
}
