param application string
param stateStoreName string

resource statestore 'radius.dev/Application/dapr.io.StateStore@v1alpha3' = {
  name: '${application}/${stateStoreName}'
  properties: {
    kind: 'state.redis'
    managed: true
  }
}

output statestore resource = statestore
