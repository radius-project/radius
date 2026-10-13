extension radius

@description('Specifies the image used by the connection-checking containers.')
param magpieimage string

@description('Specifies the published resource-types-contrib Recipe tag.')
param recipeTag string = 'edge'

var applicationName = 'corerp-container-managed-secret'
var environmentName = '${applicationName}-env'
var namespace = applicationName

resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'managed-secret-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/kube-recipes/containers:${recipeTag}'
      }
      'Radius.Data/redisCaches': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/kube-recipes/rediscaches:${recipeTag}'
      }
      'Radius.Security/secrets': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/kube-recipes/secrets:${recipeTag}'
      }
    }
  }
}

resource environment 'Radius.Core/environments@2025-08-01-preview' = {
  name: environmentName
  location: 'global'
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: namespace
      }
    }
  }
}

resource application 'Radius.Core/applications@2025-08-01-preview' = {
  name: applicationName
  location: 'global'
  properties: {
    environment: environment.id
  }
}

resource redis 'Radius.Data/redisCaches@2025-08-01-preview' = {
  name: 'managed-redis'
  location: 'global'
  properties: {
    application: application.id
    environment: environment.id
  }
}

resource consumer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'secret-consumer'
  location: 'global'
  properties: {
    application: application.id
    environment: environment.id
    containers: {
      connectioncheck: {
        initContainer: true
        image: magpieimage
        command: [
          '/bin/sh'
          '-c'
        ]
        args: [
          '''
            test -n "$CONNECTION_REDIS_HOST" || { echo "CONNECTION_REDIS_HOST is empty" >&2; exit 1; }
            test "$CONNECTION_REDIS_PORT" = "6379" || { echo "CONNECTION_REDIS_PORT is unexpected" >&2; exit 1; }
            test -n "$CONNECTION_REDIS_URL" || { echo "CONNECTION_REDIS_URL is empty" >&2; exit 1; }
            case "$CONNECTION_REDIS_URL" in redis://*) ;; *) echo "CONNECTION_REDIS_URL has unexpected scheme" >&2; exit 1 ;; esac
            echo managed-secret-init-ready
          '''
        ]
      }
      app: {
        image: magpieimage
        command: [
          '/bin/sh'
          '-c'
        ]
        args: [
          '''
            test -n "$CONNECTION_REDIS_HOST" || { echo "CONNECTION_REDIS_HOST is empty" >&2; exit 1; }
            test "$CONNECTION_REDIS_PORT" = "6379" || { echo "CONNECTION_REDIS_PORT is unexpected" >&2; exit 1; }
            test -n "$CONNECTION_REDIS_URL" || { echo "CONNECTION_REDIS_URL is empty" >&2; exit 1; }
            case "$CONNECTION_REDIS_URL" in redis://*) ;; *) echo "CONNECTION_REDIS_URL has unexpected scheme" >&2; exit 1 ;; esac
            echo managed-secret-connection-ready
            while true; do sleep 30; done
          '''
        ]
      }
    }
    connections: {
      redis: {
        source: redis.id
      }
    }
  }
}
