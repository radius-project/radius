extension radius

// No test creates this namespace, so the environment deployment must fail.
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'recipepacks-test-env-missing-namespace'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'recipepacks-missing-ns'
      }
    }
  }
}
