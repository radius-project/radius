import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-optional_port_scheme'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-optional_port_scheme'
      }
    ]
  }
}

// the container resource should use the optional port, protocol, and scheme variables if specified.
resource containera 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containera'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 4000
          port: 443 // optional: only needs to be set when a value different from containerPort is desired
          protocol: 'TCP' // optional: defaults to TCP
          scheme: 'https' // optional: used to build URLs, defaults to http or https based on port
        }
      }
      
    }
  }
}

// the container resource should use the optional port, protocol, and scheme variables if specified.
resource containerb 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerb'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          port: 80 // optional: only needs to be set when a value different from containerPort is desired
          protocol: 'TCP' // optional: defaults to TCP
          scheme: 'http' // optional: used to build URLs, defaults to http or https based on port
        }
      }
      
    }
  }
}

// the container resource should still expose a port on the containerPort if the optional variables are not specified.
resource containerc 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerc'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
        }
      }
      
    }
  }
}
