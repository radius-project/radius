extension radius

param name string
param envId string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: '${name}-app'
  properties: {
    environment: envId
  }
}

output appId string = app.id
