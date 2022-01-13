resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-module'

	resource outsideContainer 'Container' = {
		name: 'busybox'
		properties: {
			container: {
				image: 'busybox:latest'
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
