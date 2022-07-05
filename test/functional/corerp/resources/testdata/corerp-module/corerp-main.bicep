import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' 

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string = 'test'

module container 'corerp-module.bicep' = {
  name: 'nginx'
  params: {
    app: app
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-app-env-app'
  location: location
  properties: {
    environment: environment
  }
}

resource outsideContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
	name: 'busybox'
	properties: {
		container: {
			image: magpieimage
			env: {
				TEST: '${container.outputs.test.id}'
			}
		}
	}
}
