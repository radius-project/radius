resource app 'radius.dev/Application@v1alpha3' = {
  name: 'text-translation-app'

  resource store 'ContainerComponent' = {
    name: 'translation-service'
    properties: {
      container: {
        image: 'registry/container:tag'
      }
      connections: {
        translationresource: {
          kind:'azure'
          source: cognitiveServicesAccount.id
          role: [
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
    name: 'F0'
  }
}

