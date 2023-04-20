import radius as radius

param magpieimage string
param environment string 

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-extender'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource twilio 'Applications.Link/extenders@2023-04-15-preview' = {
  name: 'extr-twilio'
  location: 'global'
  properties: {
    environment: environment
    fromNumber: '222-222-2222'
    secrets: {
      accountSid: 'sid'
      authToken: 'token'
    }
  }
}

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'extr-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        TWILIO_NUMBER: twilio.properties.fromNumber
        TWILIO_SID: twilio.secrets('accountSid')
        TWILIO_ACCOUNT: twilio.secrets('authToken')
      }
    }
    connections: {}
  }
}
