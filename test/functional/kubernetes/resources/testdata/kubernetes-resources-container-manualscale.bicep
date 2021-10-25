resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-container-manualscale'

  resource frontendhttp 'HttpRoute' = {
    name: 'frontend'
  }

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      connections: {
        backend: {
          kind: 'Http'
          source: backendhttp.id
        }
      }
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: frontendhttp.id
          }
        }
        volumes:{
          'my-volume':{
            kind: 'ephemeral'
            mountPath:'/tmpfs'
            managedStore:'memory'
          }
          'my-volume2':{
            kind: 'persistent'
            mountPath:'/tmpfs2'
            source: myshare.id
            rbac: 'read'
          }
        }
        env: {
          SERVICE__BACKEND__HOST: backendhttp.properties.host
          SERVICE__BACKEND__PORT: '${backendhttp.properties.port}'
        }
      }
      traits: [
        {
          kind: 'radius.dev/ManualScaling@v1alpha1'
          replicas: 2
        }
      ]
    }
  }

  resource myshare 'Volume' = {
    name: 'myshare'
    properties:{
      kind: 'azure.com.fileshare'
      managed:true
    }
  }

  resource backendhttp 'HttpRoute' = {
    name: 'backend'
  }

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'rynowak/backend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: backendhttp.id
          }
        }
      }
    }
  }
}
