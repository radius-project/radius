# User-Defined Types Schema Validation

* **Author**: @nithyatsu

## Overview

The [user-defined-types](https://github.com/radius-project/design-notes/blob/main/architecture/2024-07-user-defined-types.md) feature enables end-users to define their own resource types as part of their tenant of Radius. User-defined types have the same set of capabilities and enable the same features (connections, app graph, recipes, etc) as system-defined types in Radius. 

User-defined types are created with rad resource-type create command, which takes a resource type manifest file as input. Here is a very simple resource manifest -

 ```yaml
name: 'Mycompany.Messaging'
types:
    plaidResource:
      apiVersions:
        "2023-10-01-preview":
          schema:
            openAPIV3Schema:
              type: object
              properties:
                host:
                  type: string
                  description: hostname 
                port:
                  type: string
                  description: port
              required:
              - host
              - port                  
      capabilities: []
```

The `schema` serves as a contract, defining what properties developers are allowed to set and what data is provided to applications. Essentially, the schema of a resource type is its API.

The ability to define schema allows for differentiation and customization. For instance, in case of a databse UDT, an organization might use t-shirt sizes to describe the storage capacity of a database (e.g., S, M, L) or define their own vocabulary for fault domains (e.g., zonal, regional).

Technically, defining schemas is complex. It should be described with OpenAPI,  which represents the state of the art but is a challenging tool to wield. It includes many constructs that are not conducive to good API design. Consequently, every project leveraging OpenAPI tends to define its own supported subset of its functionality.

This document summarizes the key decisions that Radius makes on what constitutes a valid UDT schema.

## Terms and definitions

*Please read the [Radius API](https://docs.radapp.io/concepts/technical/api/) conceptual documentation. This document will heavily use the terminology and concepts defined there.*

|Term| Definition|                                                              
| ----------------------------- | ------------------------------------------|
| User-defined type | A resource-type that can be defined or modified by end-users.|
| OpenAPI/Schema | A format for documenting HTTP-based APIs. In this document we're primarily concerned with the parts of OpenAPI pertaining to request/response bodies.   |                                                                     

## Objectives

https://github.com/radius-project/radius/issues/6688

### Guiding principles

We begin with an initial subset of OpenAPI features that Radius UDTs would support. Depending on user input, we would add more fatures in subsequent iterations.

### Goals

Summarize the initial subset of OpenAPI features that Radius would support for user-defined types. 

### Non goals

Validation of a UDT resource against its UDT resource type schema
  
### User scenarios 

As a cloud operations engineer, I am responsible for ensuring the deployment of databases. The permissible sizes of the databases vary depending on whether they are intended for development or production environments. It is crucial to prevent development engineers from provisioning databases with excessive resources.

## User Experience (if applicable)

Users author a manifest like the following to defind a user-defined resource type.

**Sample Input:**

```yaml
namespace: MyCompany.Resources
types:
  postgresDatabases:
    description: A postgreSQL database
    apiVersions:
      '2025-01-01-preview':
        schema: 
          type: object
          properties:
            size:
              type: string
              description: |
                The size of database to provision:
                  - 'S': 0.5 vCPU, 2 GiB memory, 20 GiB storage
                  - 'M': 1 vCPU, 4 GiB memory, 40 GiB storage
                  - 'L': 2 vCPU, 8 GiB memory, 60 GiB storage
                  - 'XL': 4 vCPU, 16 GiB memory, 100 GiB storage
              enum:
                - S
                - M
                - L
                - XL
            logging-verbosity:
              type: string
              description: >
                The logging level for the database:
                  - 'TERSE': Not recommended; does not provide guidance on what to do about an error
                  - 'DEFAULT': Recommended level
                  - 'VERBOSE': Use only if you plan to actually look up the Postgres source code
              enum:
                - TERSE
                - DEFAULT
                - VERBOSE
            connection-string:
              type: string
              readOnly: true
              description: 'Fully qualified string to connect to the resource'
              env-variable: POSTGRESQL_CONNECTION_STRING
            credentials:
              type: object
              readOnly: true
              properties:
                username:
                  type: string
                  description: 'Username for the database'
                  env-variable: POSTGRESQL_USERNAME
                password:
                  type: string
                  description: 'Password for the database user'
                  env-variable: POSTGRESQL_PASSWORD
          required:
            - size
```
**Sample Output:**

Users can use this schema to generate a Bicep extension providing strongly-typed editor support. Here's an example of Bicep code that matches this API definition.

```bicep
extension radius
extension mycompany // generated from the schema

resource webapp 'Applications.Core/containers@2024-01-01' = {
  name: 'sample-webapp'
  properties: {
    image: '...'
    env: {
      // Bicep editor has completion for these 
      DB_HOSTNAME: { fromValue: { value: db.properties.binding.hostname } }
      DB_USERNAME: { fromValue: { value: db.properties.binding.username } }
      DB_HOSTNAME: {
        fromSecret: {
          secret: db.properties.binding.secret 
          key: 'password'
        }
      }
    }
  }
}

resource db 'MyCompany.Resources/postgresDatabases@2025-01-01-preview' = {
  name: 'sample-db'
  properties: {
    size: 'L' // Bicep editor can validate this field
  }
}
```

## Design

When user executes a `rad resource-type create` command, the cli should validate the schema object provided in the manifest and report any errors back to the users before making the API call. 

The server side, upon receiving a resource payload for a resource-type resource, must again validate the schema before it saves the resource into database.

We should implement validations on both client and server since rad cli need not be the only client. It is important to validate payloads since these can be any arbitrary data.

### High Level Design

We expect users will provide the structural schema for new resource types and describe it use using [Open API v3](https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.2.md#schema-object). Structural schema ensures explicit type definition for properties and enforces strict validation rules.


### Architecture Diagram

NA 

### Detailed Design

#### Structure

Users provide us with the schema of UDT by defining `properties` in `openAPIV3Schema`. `openAPIV3Schema` is an `object` that holds properties of UDT. There is no support for defining custom fields outside of `properties`.

Each of the property is a user defined property of the UDT. It has a type and optional description and format. The type can be 

* scalar

We support all of the scalars supported by OpenAPI as documented in https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.2.md#data-types. 


https://github.com/readmeio/oas-examples/blob/a331c65623f795af68602dd6f02e116f905d9297/3.0/yaml/schema-types.yaml details several examples covering the options available.
  
```
schema: 
  openAPIV3Schema:
    type: object
    properties:
      size:
        type: string  
        description: The size of database to provision
        enum:
        - S
        - M
        - L
        - XL
      host:
        type: string
        description: hostname
        maxLength: 20
```

* array

`array` must specify a `type` for item

```
schema:
  openAPIV3Schema: 
    type: object
    properties:
      ports:
        type: array
        description: "ports this resource binds to"
        item:
          type: integer
          format: uint32
```

* map

We support map through `adddiionalProperties`. This is useful when the resource type allows for dynamic(user defined) keys. We still must specify a type for the value of the property.

```
schema:
  openAPIV3Schema:
    type: object
    properties:
      name:
        type: string
        description: The name of the resource
      labels:
        type: object
        description: A map of labels for the resource
        additionalProperties:
          type: string
```


# Scalars

Schemas will support every construct OpenAPI provides for a scalar field. That includes all of the modifiers like `enum` and `format` and also all of the validation attributes like `minLength`.

# Maps

Schemas support maps using `additional properties`. The additionalProperties keyword allows for dynamic keys that are not predefined in the schema
The Schema is allowed to have either user-defined properties or additional properties which specifies a type. The type can be `Object` or another scalar.

# Arrays

Arrays must specify a type for `item`.

# References

Schemas are allowed to reference reusable built-in schemas of Radius using the `$ref` construct of OpenAPI. Example:`$ref: 'https://radapp.io/schemas/v1#RecipeStatus'` can reference the schema for a "recipe status". This aids with consistency and reduces boilerplate for users. 

The exact URL and set of types that can be referenced is TBD.


# Limitations

All the limitations here are because at this point, we want to limit complexity and keep our tooling simple. Based on feedback, we would likely add support for some of the more complex OpenAPI features. 

1. We are not supporting inheritence and polymorphism. Objects may not use the following constructs:

- `allOf`
- `anyOf`
- `oneOf`
- `not`
- `discriminator`

2. Objects may not set both `additionalProperties` as well as  define their own properties.

3. Schemas are not allowed to use `$ref` to reference other than what we provide in Radius. This reduces concept count and simplifies our tooling. We can reconsider this based on feedback. This also prevents the definition of recursive types and circular references. 

4.  readOnly: true => user cant set this property, its available as output


# Radius specific schema attributes:

Typically, "recipes" block provides details on recipe used by a specific type. 
Example:
```
recipe: {
      name: 'default'
      parameters: {
        redis_cache_name: redisCacheName
      }
    }
```
- `name` is the name of recipe for the type which should be used in this  deployment.
- `parameters` get passed to recipe. 

However, we are choosing to have only the default recipe for each UDT type. 
We are also choosing to pass all the properties in schema to recipe.

If this is finalized, then we do not need recipe construct in schema for UDT. 
However, we have to find ways to "mark" a type as UDT or revisit/ reimplement existing design.

### Implementation Details

N/A for this document. This is a spec. 

### Error Handling

N/A for this document. This is a spec. 

## Test plan

N/A for this document. This is a spec. 

## Security

N/A

## Compatibility (optional)


## Monitoring and Logging


## Development plan

## References
https://github.com/readmeio/oas-examples/tree/main/3.0/yaml
https://kubernetes.io/blog/2019/06/20/crd-structural-schema/
https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.2.md#properties
