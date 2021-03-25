## Radius Application Model Schemas

This directory holds the JSON schema documents used to describe types in the Radius Application Model.

These artifacts are not currently consumed directly by any tools (yet). In the near future these will drive:

- The Bicep compiler's type validation
- Validation of API request payloads
- Documentation for the Application Model

> 💡 The Open API documents may reference types defined here for reuse of definitions. You should update the generated code (run `make generate`) when making **any** schema change.

## What these types represent

The schemas here represent the Radius Application model as it would be typed by a user in Bicep code. They do not include ARM-isms that won't be reflected in a user's code.

Here's an example of what a user types:

```txt
...
    resource frontend 'Components' = {
      name: 'frontend
      kind: 'radius.dev/Container@v1alpha1'
      properties: {
        run: {
          container: {
            image: 'rynowak/frontend:latest'
          }
        }
      }
    }
...
```

Here's an example of what an ARM resource for the same thing looks like:

```txt
...
    {
      "id": " ..... ",
      "name": "frontend",
      "properties": {
        "provisioningState": "Succeeded",
        "revision": "c3036974529c68c1d9f9c7e98bc7d986b8c4daa8",
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

If you look past the format, you can see that there are additional properties that reflect ARM's semantics:

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

