# Introduction

There are two kind of JSON-based specifications we use in Radius today. The reasons for _two_ are:
* OpenAPI describes an API, which by nature will need to describe the types used by such API. However:
- Much focus is in generating the model types, the server stub, or the client library.  For example, we use Azure Autorest to generate a client library from an OpenAPI spec.
- Very little support for standalone validation. The closest library that support such is https://github.com/go-swagger/go-swagger but (1) its error messages are terrible and  (2) they are looking for a new maintainer.
* JSON schema's main focus is validation. There is a lot of support to validate a JSON blob directly. However, since they aren't for defining APIs, we can't use a client code generator like Azure Autorest with JSON Schema.

The good news is that we are able to generate the OpenAPI spec from our JSON schema, thanks to the fact that we don't use obscure features in JSON Schema.

## JSON Schema

### What is it?
JSON Schema is a vocabulary that allows you to annotate and validate JSON documents.

### How do we use it?

Take a look at `/pkg/radrp/schemas`. We are using this to:
* Validate JSON seen by the RP,
* Validate JSON in the K8s webhook,
* Validate JSON in the rad-bicep compiler, and
* Generate the Open API spec.

For more information about our generator for Open API spec, see later section about the whole schema-based generation process in Radius.

## Open API Spec

### What is it?
The OpenAPI Specification (OAS) defines a standard, language-agnostic interface to RESTful APIs which allows both humans and computers to discover and understand the capabilities of the service without access to source code, documentation, or through network traffic inspection. When properly defined, a consumer can understand and interact with the remote service with a minimal amount of implementation logic.

### How do we use it?

Take a look at `/schemas/rest-api-spec`. We are using this to:
* To generate `pkg/radclient` using Azure Autorest (aka `make generate-radclient`)
* To generate model objects to deserialized JSON from the Resource Provider side
* To generate model objects to be persisted in our Database]
* To generate documentation for our Resource Provider

# Schema-based code generation process

## Step 1: Generating OpenAPI v2 spec from our JSON Schema

In this step, all the resource schemas in /pkg/radrp/schemas is consumed, and for each resource type we use the [Go Template](https://github.com/project-radius/radius/blob/main/pkg/tools/codegen/schema/resource_boilerplate.json) to generate:
- The corresponding List type, which is a boilerplate container type, and
- The corresponding API calls GET, PUT, PATCH etc... for each resource.

After that, we append [the header Go template template](https://github.com/project-radius/radius/blob/main/pkg/tools/codegen/schema/boilerplate.json).

To execute this code generation step, run `make generate-openapi-specs` (or simply do`make generate` to run all the code generation

## Step 2: Generating the Azure Autorest client from the OpenAPI v2 spec

This steps make use of the https://github.com/azure/autorest project to generate Go client in `pkg/azure/clients/radclient.

To execute this code generation step, run `make generate-radclient`.



# Appendix: Similarity between Open API v2 and JSON Schema Draft 4

The main similiarity is how types are defined in JSON schema. A general structure is this
```json
{
  "definitions": {
    "MyAweSomeType": {
      "type": "object",
      "description": "Description goes here. This is super useful because JSONdoesn't have comments 🤯!",
      "properties": {
        "firstField": {
          "description": "First field's description",
          "type": "string",
        },
        "secondField": {
          "description": ".",
          "$ref": "#/definitions/AnotherType"
        },
        "others": {
		  "...": "..."
	    }
      },
    },
  }
}
```

Take a look at `/schemas/rest-api-spec/radius.json`'s `"definitions"` section to have a general understanding of the syntax.

# Differences (that we care about)

## Discriminated unions:

### What is it?

It is a simple way of allowing polymorphism using a "discriminator" field. For examples, we will have different spec of a `ComponentTrait` depending on the `ComponentTrait#Kind` field:
- Kind == "dapr.io/Sidecar@v1alpha1" => assuming `DaprComponentTrait`
- Kind == "radius.dev/ManualScaling@v1alpha1" => assuming `ManualScalingTrait`.

### Discrimated union in JSON Schema
This is very often done using the `oneOf` syntax, which is supported in both OpenAPI and JSON Schema:
```json
{
  "definitions": {
    "ComponentTrait": {
      "description": "Trait of a component.",
      "type": "object",
      "oneOf": [{
        "$ref": "#/definitions/DaprTrait"
      }, {
        "$ref": "#/definitions/ManualScalingTrait"
      }]
    },
    "DaprTrait": {
      "type": "object",
      "properties": {
        "kind": {
          "type": "string",
          "enum": ["dapr.io/Sidecar@v1alpha1"]
        },
      },
    },
    "ManualScalingTrait": {
      "type": "object",
      "properties": {
        "kind": {
          "type": "string",
          "enum": ["radius.dev/ManualScaling@v1alpha1"]
        },
      },
    }
  }
}
```

### Discrimated union supported in Azure Autorest
*However*, Azure Autorest does not support `oneOf`. There way of defining discrimated union is based on `allOf` and the `x-ms-discrimator-value`, specifying from the child types. So we end up with an OpenAPI spec that looks slightly different from our JSON Schema:

```json
{
  "swagger": "2.0",
  "info": {
    "version": "2.0",
    "title": "Common types"
  },
  "paths": {},
  "definitions": {
    "ComponentTrait": {
      "type": "object",
      "description": "Trait of a component.",
      "properties": {
        "kind": {
          "description": "Trait kind.",
          "type": "string"
        }
      },
      "required": [
        "kind"
      ],
      "discriminator": "kind"
    },
    "DaprTrait": {
      "type": "object",
      "allOf": [{"$ref": "#/definitions/ComponentTrait"}],
      "x-ms-discriminator-value": "dapr.io/Sidecar@v1alpha1",
    },
    "ManualScalingTrait": {
      "type": "object",
      "description": "ManualScaling ComponentTrait",
      "allOf": [{"$ref": "#/definitions/ComponentTrait"}],
      "x-ms-discriminator-value": "radius.dev/ManualScaling@v1alpha1",
    }
  }
}
```

## `additionalProperties: Foo` where Foo can be primitives or Object

For both JSON Schema and OpenAPI spec type definitions:
- `additionalProperties: false` => Does not allow any additional properties.
- `additionalProperties: type` => Allow additional properties of the same types. Azure Autorest will generate a model type with `AdditionalProperties map[string]type`. This struct will contain all of the left-over values unspecified in the `properties` list.

For JSON Schema:
- `additionalProperties: true` => Allow additional properties of _any kind_. This option is not allowed in Open API v2.0.

However, due to the lack of `anyOf` support in Azure Autorest, we can generate something like:
* `additionalProperties: int` => `AdditionalProperties map[string]int`,
* `additionalProperties: string` => `AdditionalProperties map[string]string`, or
* `additionalProperties: object` => `AdditionalProperties map[string]map[string]interface{}
but not something like
* `additionalProperties: any` => `AdditionalProperties map[string]interface{}`

Two ways we can work around this:
1. We don't declare a OpenAPI spec for any object requiring a freeform `AdditionalProperties` field, or
2. We use discriminated union to completely remove the need for `AdditionalProperties` field.
