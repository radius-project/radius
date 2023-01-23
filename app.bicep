import radius as radius
param application string

resource demo 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'demo'
  location: 'global'
  properties: {
    application: application
    container: {
      image: 'radius.azurecr.io/tutorial/webapp:edge'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
  }
}
