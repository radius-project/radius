provider radius

param magpieimage string
param environment string 
param location string = 'global'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-extender'
  location: location
  properties: {
    environment: environment
  }
}

resource twilio 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'extr-twilio'
  properties: {
    environment: environment
    fromNumber: '222-222-2222'
    secrets: {
      accountSid: 'sid'
      authToken: 'token'
    }
    resourceProvisioning: 'manual'
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'extr-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        TWILIO_NUMBER: twilio.properties.fromNumber
        TWILIO_SID: twilio.listSecrets().accountSid
        TWILIO_ACCOUNT: twilio.listSecrets().authToken
      }
    }
    connections: {}
  }
}
