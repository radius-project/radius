resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-statestore-generic'

  //SAMPLE
  resource statestore 'dapr.io.StateStore@v1alpha3' = {
    name: 'statestore'
    properties: {
      kind: 'generic'
      type: 'state.couchbase'
      metadata: {
        couchbaseURL: '***'
        username: 'admin'
        password: '***'
        bucketName: 'dapr'
      }
      version: 'v1'
    }
  }
  //SAMPLE
}
