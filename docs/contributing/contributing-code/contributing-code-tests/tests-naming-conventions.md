# Radius functional test naming conventions
Today, the [functional tests](https://github.com/radius-project/radius/tree/main/test/functional/shared/resources) use some abbreviations to make sure that the resource labels will not exceed 63 characters (application + resource name). Here are a list of some of the abbreviations and patterns:

|Keyword|Abbreviation|
|-|-|
|application|app|
|environment|env|
|container|ctnr|
|gateway|gtwy|
|httpRoute|rte|
|generic|gnrc|
|servicebus|sb|
|secretstore|scs|
|statestore|sts|
|tablestorage|ts|
|extender|extr|
|database|db|
|mongodb|mdb|
|rabbitmq|rmq|
|redis|rds|
|frontend|front|
|backend|back|
|usersecrets|us|

## Patterns
### Prefixes
In any new test, the resources should be prefixed with the test type:
#### corerp-resources-gateway.bicep
```bicep
resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'gtwy-gtwy'
  ...
}

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'gtwy-front-rte'
  ...
}
```
This specifies `gtwy` as the test type and the suffix declares the type of resource, either *gateway* or *frontend route*.

### Magpie
Many of the tests use the `magpiego` image as a testing container. These should be named with the test type as the prefix and `app-ctnr` as the suffix:
#### corerp-resources-mongodb.bicep
```bicep
resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mdb-app-ctnr'
  ...
}
```
