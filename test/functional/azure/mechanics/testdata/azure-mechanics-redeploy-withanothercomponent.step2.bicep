resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-mechanics-redeploy-withantothercomponent'

  resource a 'Components' = {
    name: 'a'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/guineapig'
        }
      }
    }
  }

  resource b 'Components' = {
    name: 'b'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/guineapig'
        }
      }
    }
  }
}
