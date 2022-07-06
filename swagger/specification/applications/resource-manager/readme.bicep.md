##Bicep

### Bicep multi-api
```yaml $(bicep) && $(multiapi)
batch:
  - tag: applications.core-2022-03-15-privatepreview
  - tag: applications.connector-2022-03-15-privatepreview

```
### Tag: applications.core-2022-03-15-privatepreview and bicep
```yaml $(tag) == 'applications.core-2022-03-15-privatepreview' && $(bicep)
input-file:
  - Applications.Core/preview/2022-03-15-privatepreview/global.json
  - Applications.Core/preview/2022-03-15-privatepreview/environments.json
  - Applications.Core/preview/2022-03-15-privatepreview/applications.json
  - Applications.Core/preview/2022-03-15-privatepreview/httpRoutes.json
  - Applications.Core/preview/2022-03-15-privatepreview/gateways.json
  - Applications.Core/preview/2022-03-15-privatepreview/containers.json

```
### Tag: applications.connector-2022-03-15-privatepreview and bicep
```yaml $(tag) == 'applications.connector-2022-03-15-privatepreview' && $(bicep)
input-file:
  - Applications.Connector/preview/2022-03-15-privatepreview/global.json
  - Applications.Connector/preview/2022-03-15-privatepreview/mongoDatabases.json
  - Applications.Connector/preview/2022-03-15-privatepreview/daprPubSubBrokers.json
  - Applications.Connector/preview/2022-03-15-privatepreview/sqlDatabases.json
  - Applications.Connector/preview/2022-03-15-privatepreview/redisCaches.json
  - Applications.Connector/preview/2022-03-15-privatepreview/rabbitMQMessageQueues.json
  - Applications.Connector/preview/2022-03-15-privatepreview/daprSecretStores.json
  - Applications.Connector/preview/2022-03-15-privatepreview/daprInvokeHttpRoutes.json
  - Applications.Connector/preview/2022-03-15-privatepreview/daprStateStores.json
  - Applications.Connector/preview/2022-03-15-privatepreview/extenders.json

```
