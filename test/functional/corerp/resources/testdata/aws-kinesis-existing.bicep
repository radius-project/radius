import aws as aws

param streamName string

import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource stream 'AWS.Kinesis/Stream@default' existing = {
  alias: streamName
  properties: {
    Name: streamName
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'aws-kinesis-existing-app'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'aws-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        TEST: stream.properties.Name
      }
      ports: {
        web: {
          containerPort: port
        }
      }
    }
    connections: {}
  }
}

