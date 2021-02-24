application app = {
  name: 'frontend-backend'

  // deploy two containers
  // - frontend: rynowak/frontend:0.5.0-dev
  // - backend: rynowak/backend:0.5.0-dev
  //
  // need to communicate via HTTP from frontend->backend and pass the URL to frontend with env-vars:
  // - SERVICE__BACKEND__HOST
  // - SERVICE__BACKEND__PORT

  instance frontend 'radius.dev/Container@v1alpha1' = {
    name: 'frontend'
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
    }
  }

  instance backend 'radius.dev/Container@v1alpha1' = {
    name: 'backend'
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