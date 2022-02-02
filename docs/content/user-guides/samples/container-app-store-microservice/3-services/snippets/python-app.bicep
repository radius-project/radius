resource python_app 'Container' = {
  name: 'python-app'
  properties: {
    connections: {
      kind: {
        kind: 'dapr.io/StateStore'
        source: statestore.id
      }
    }
    container: {
      image: python_service_build.image
      ports: {
        web: {
          containerPort: 5000
        }
      }
    }
    traits: [
      {
        kind: 'dapr.io/Sidecar@v1alpha1'
        appId: 'python-app'
        appPort: 5000
        provides: python_app_route.id
      }
    ]
  }
}
