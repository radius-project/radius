import kubernetes from kubernetes

resource pod 'kubernetes.core/Pod@v1' = {
  metadata: {
    name: 'mypod'
  }
  spec: {
    containers: [
      {
        name: 'web-server'
        image: 'nginx:latest'
        ports: [
          {
            containerPort: 80
          }
        ]
      }
    ]
  }
}
