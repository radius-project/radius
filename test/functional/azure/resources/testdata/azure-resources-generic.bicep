resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-generic'
  resource twilio 'Generic@v1alpha3' = {
    name: 'twilio'
    properties: {
      properties: {
        fromNumber: '222-222-2222'
      }
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
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          TWILIO_NUMBER: twilio.properties.fromNumber
          TWILIO_SID: twilio.secrets('accountSid')
          TWILIO_ACCOUNT: twilio.secrets('authToken')
        }
      }
    }
  }
}
