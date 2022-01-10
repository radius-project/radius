resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-module'
}

module container 'module.bicep' = {
  name: 'nginx'
  params: {
    app: app
  }
}
