import radius as radius

param environment string 

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'connectorrp-resources-extender'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource twilio 'Applications.Connector/extenders@2022-03-15-privatepreview' = {
  name: 'twilio'
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
