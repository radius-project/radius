extension testresources
extension radius
extension kubernetes with {
  kubeConfig: ''
  namespace: 'udt-externalresource-app'
} as kubernetes

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udt-externalresource-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udt-externalresource-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udt-externalresource-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource externalresourcecntr 'Applications.Core/containers@2023-10-01-preview' = {
    name: 'externalresourcecntr'
    properties: {
      application: app.id
      container: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo hello; sleep 10;done']
        // env: {
        //   UDTCONFIGMAP_DATA: {
        //     value: externalresource.properties.configMap
        //   }          
        // }
    }
    connections: {
      externalresource: {
        source: externalresource.id
      }
    }
  }
}

resource externalresource 'Test.Resources/externalResource@2023-10-01-preview' = {
  name: 'udt-externalresource'
  location: location
  properties: {
    application: app.id
    environment: env.id
    configMap: string(configMap.data)
  }
}

resource configMap 'core/ConfigMap@v1' = {
  metadata: {
    name: 'udt-config-map'
  }
  data: {
    'app1.sample.properties': 'property1=value1\nproperty2=value2'
    'app2.sample.properties': 'property3=value3\nproperty4=value4'
  }
}
