resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-connection-cognitive-service'

  resource store 'Container' = {
    name: 'translation-service'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        translationresource: {
          kind:'azure'
          source: cognitiveServicesAccount.id
          roles: [
            'Cognitive Services User'
          ]
        }
      }
    }
  }
}

resource cognitiveServicesAccount 'Microsoft.CognitiveServices/accounts@2017-04-18' = {
  name: 'TextTranslationAccount-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'TextTranslation'
  sku: {
    name: 'S1'
  }
}

