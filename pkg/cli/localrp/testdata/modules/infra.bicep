import kubernetes from kubernetes

param application string
param computedValue string

resource secret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'secret'
  }
}

resource db 'radius.dev/Application/mongo.com.MongoDatabase@v1alpha3' = {
  name: '${application}/db'
  properties: {
    secrets: {
      connectionString: base64ToString(secret.data.connectionString)
    }
  }
}

output db resource 'radius.dev/Application/mongo.com.MongoDatabase@v1alpha3' = db
output computedValue string = computedValue
