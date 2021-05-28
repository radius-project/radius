resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'inbound-route'

  // deploy two containers
  // - frontend: rynowak/frontend:0.5.0-dev
  // - backend: rynowak/backend:0.5.0-dev
  //
  // - frontend is exposed to internet traffic
  // - backend is not exposed to internet traffic
  //
  // need to communicate via HTTP from frontend->backend and pass the URL to frontend with env-vars:
  // - SERVICE__BACKEND__HOST
  // - SERVICE__BACKEND__PORT

  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/frontend:0.5.0-dev'
        }
      }
      dependsOn: [
        {
          name: 'backend'
          kind: 'http'
          setEnv: {
            SERVICE__BACKEND__HOST: 'host'
            SERVICE__BACKEND__PORT: 'port'
          }
        }
      ]
      provides: [
        {
          name: 'frontend'
          kind: 'http'
          containerPort: 80
        }
      ]
      traits: [
        {
          kind: 'radius.dev/InboundRoute@v1alpha1'
          properties: {
            service: 'frontend'
          }
        }
      ]
    }
  }

  resource backend 'Components' = {
    name: 'backend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/backend:0.5.0-dev'
        }
      }
      provides: [
        {
          name: 'backend'
          kind: 'http'
          containerPort: 80
        }
      ]
    }
  }
}
