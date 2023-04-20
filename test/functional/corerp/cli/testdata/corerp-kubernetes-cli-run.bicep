import radius as radius

param application string

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'k8s-cli-run-logger'
  location: 'global'
  properties: {
    application: application
    container: {
      image: 'debian'
      command: ['/bin/sh']

      // The test looks for this specific output, keep in sync with the CLI run test!
      args: ['-c', 'while true; do echo "hello from the streaming logs!"; sleep 10;done']
    }
  }
}
