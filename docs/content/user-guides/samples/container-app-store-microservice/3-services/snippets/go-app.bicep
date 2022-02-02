resource go_app 'Container' = {
  name: 'go-app'
  properties: {
    container: {
      image: go_service_build.image
      ports: {
        web: {
          containerPort: 8050
        }
      }
    }
    traits: [
      {
        kind: 'dapr.io/Sidecar@v1alpha1'
        appId: 'go-app'
        appPort: 8050
        provides: go_app_route.id
      }
    ]
  }
}
