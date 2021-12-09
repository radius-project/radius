@description('SKU for Text Translation API')
param SKU string = 'F0'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'text-translation-app'

  resource store 'ContainerComponent' = {
    name: 'translation-service'
    properties: {
      container: {
        image: '<container_image>'
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
    name: SKU
  }
  properties: {}
}
