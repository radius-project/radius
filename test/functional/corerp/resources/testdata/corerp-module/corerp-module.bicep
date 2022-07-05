import radius as radius

param app resource 'Applications.Core/applications@2022-03-15-privatepreview'

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: '${app.name}/container'
  properties: {
    container: {
      image: 'nginx:latest'
    }
  }
}

resource backendhttp 'Applications.Core/httproutes@2022-03-15-privatepreview' = {
  name: '${app.name}/backendhttp'
}

output test resource 'Applications.Core/httproutes@2022-03-15-privatepreview' = backendhttp
