param app object

param name string
param image string
param ports object
param livenessPath string = '/healthz'
param livenessPort int = 3000

param connections object

resource myapp 'radius.dev/Application@v1alpha3' existing = {
  name: app.name

  resource container 'Container' = {
    name: name
    properties: {
      container: {
        image: image
        ports: ports
        livenessProbe: {
          kind: 'httpGet'
          path: livenessPath
          containerPort: livenessPort
        }
      }
      connections: connections
    }
  }

}

output container object = myapp::container
