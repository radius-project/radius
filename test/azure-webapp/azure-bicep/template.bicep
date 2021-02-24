application app = {
  name: 'azure-webapp'

  // Using a similar schema to k4se
  instance webapp 'azure.com/WebApp@v1alpha1' = {
    name: 'webapp'
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