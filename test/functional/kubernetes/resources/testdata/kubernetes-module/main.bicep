param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' string = 'radiusdev.azurecr.io/magpiego:latest' string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-module'

	resource outsideContainer 'Container' = {
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
}

module container 'module.bicep' = {
  name: 'nginx'
  params: {
    app: app
  }
}

