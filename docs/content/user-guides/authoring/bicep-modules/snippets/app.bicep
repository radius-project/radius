resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'my-application'
}

module infra 'infra.bicep' = {
  name: 'infra-module'
  params: {
    app: myapp
  }
}

module frontend 'frontend.bicep' = {
  name: 'frontend-module'
  params: {
    app: myapp
    backendHttp: backend.outputs.backendHttp
  }
}

module backend 'backend.bicep' = {
  name: 'backend-module'
  params: {
    app: myapp
    mongo: infra.outputs.mongo
  }
}
