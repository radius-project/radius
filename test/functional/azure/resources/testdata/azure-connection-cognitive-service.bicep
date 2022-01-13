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
            '25fbc0a9-bd7c-42a3-aa1a-3b75d497ee68'
            '/subscriptions/${subscription().subscriptionId}/providers/Microsoft.Authorization/roleDefinitions/9894cab4-e18a-44aa-828b-cb588cd6f2d7'
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

