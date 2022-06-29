param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-extender'
  resource twilio 'Extender@v1alpha3' = {
    name: 'twilio'
    properties: {
      fromNumber: '222-222-2222'
      secrets: {
        accountSid: 'sid'
        authToken: 'token'
      }
    }
  }

  resource myapp 'Container@v1alpha3' = {
    name: 'myapp'
    properties: {
      container: {
        image: magpieimage
        env: {
          TWILIO_NUMBER: twilio.properties.fromNumber
          TWILIO_SID: twilio.secrets('accountSid')
          TWILIO_ACCOUNT: twilio.secrets('authToken')
        }
      }
    }
  }
}
