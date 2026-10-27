extension radius

@description('Environment ID supplied by the workspace or --environment.')
@minLength(1)
param environment string

@description('Application to deploy.')
@minLength(1)
@maxLength(63)
param applicationName string = 'cli-demo'

resource application 'Radius.Core/applications@2025-08-01-preview' = {
  name: applicationName
  properties: {
    environment: environment
  }
}
