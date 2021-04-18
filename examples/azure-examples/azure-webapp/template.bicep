resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-webapp'

  // Using a similar schema to k4se
  resource webapp 'Components' = {
    name: 'webapp'
    kind: 'azure.com/WebApp@v1alpha1'
    properties: {
      run: {
        code: {
          containers: [
            {
              name: 'webapp'
              image: 'rynowak/dotnet-webapp:latest'
              livenessProbe: {
                httpGet: {
                  path: '/healthz'
                  port: 80
                }
              }
              readinessProbe: {
                httpGet: {
                  path: '/healthz'
                  port: 80
                }
              }
              ports: [
                {
                  name: 'http'
                  containerPort: 80
                }
              ]
            }
          ]
        }
      }
    }
  }
}
