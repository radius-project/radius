## Radius Application Model Schemas

This directory holds the JSON schema documents used to describe types in the Radius Application Model.

These artifacts are not currently consumed directly by any tools (yet). However they may be referened by the Open API documents for reuse of definitions. You should update the generated code when making **any** schema change

In the near future these will drive:

- The Bicep compiler's type validation
- Validation of API request payloads
- Documentation for the Application Model

## What these types represent

The schemas here represent the Radius Application model as it would be represented internally to our RP. We omit some of the properties that are required by ARM - because they are not part of our processing.

Here's an example of a payload to matches one of our schemas:

```txt
...
{
  "name": "frontend",
  "properties": {
    "run": {
      "container": {
        "image": "rynowak/frontend:0.5.0-dev"
      }
    }
  }
}
...
```

Here's an example of what an ARM response for the same component might be:

```txt
...
{
  "id": " ..... ",
  "name": "frontend",
  "properties": {
    "provisioningState": "Succeeded",
    "run": {
      "container": {
        "image": "rynowak/frontend:0.5.0-dev"
      }
    }
  },
  "resourceGroup": "rynowak-radius",
  "type": "Microsoft.CustomProviders/resourceProviders/Applications/Components"
}
...
```

You can see that the second example there are additional properties that reflect ARM's semantics:

- `id`
- `properties/provisioningState`
- `resourceGroup`
- `type`

The schemas in this section **omit** those properties.

### Structure

In general each file should contain a single API version. Right now this means that for each top level type there's a file with `v1alpha1` in the name.

We're using `http://json-schema.org/draft-04/schema#` as the JSON schema version. There are numerous different flavors of JSON schema in use, and draft 04 is best supported by ARM.

- `applications`: JSON schemas for the `Application` type
- `components`: JSON schemas for the `Component` type
- `deployments`: JSON schemas for the `Deployment` type

