import radius as radius

param magpieimage string
param environment string 

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-extender'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource twilio 'Applications.Link/extenders@2022-03-15-privatepreview' = {
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

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
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
