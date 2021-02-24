application app = {
  name: 'azure-function'

  instance function 'azure.com/Function@v1alpha1' = {
    name: 'function'
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