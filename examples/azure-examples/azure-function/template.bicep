resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-function'

  resource function 'Components' = {
    name: 'function'
    kind: 'azure.com/Function@v1alpha1'
    properties: {
      run: {
        code: {
          containers: [
            {
              name: 'function'
              image: 'rynowak/dotnet-function:latest'
            }
          ]
        }
        httpOptions: {
          appPort: 80
        }
      }
    }
  }
}
