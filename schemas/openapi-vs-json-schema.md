# Introduction

There are two kind of JSON-based specifications we use in Radius today. The reasons for _two_ are:
* OpenAPI describes an API, which by nature will need to describe the types used by such API. However:
- Much focus is in generating the model types, the server stub, or the client library.  For example, we use Azure Autorest to generate a client library from an OpenAPI spec.
- Very little support for standalone validation. The closest library that support such is https://github.com/go-swagger/go-swagger but (1) its error messages are terrible and  (2) they are looking for a new maintainer.
* JSON schema's main focus is validation. There is a lot of support to validate a JSON blob directly. However, since they aren't for defining APIs, we can't use a client code generator like Azure Autorest with JSON Schema.

## Open API Spec

### What is it?
The OpenAPI Specification (OAS) defines a standard, language-agnostic interface to RESTful APIs which allows both humans and computers to discover and understand the capabilities of the service without access to source code, documentation, or through network traffic inspection. When properly defined, a consumer can understand and interact with the remote service with a minimal amount of implementation logic.

### How do we use it?

Take a look at `/schemas/rest-api-spec`. Our plan is:
* To generate `pkg/radclient` using Azure Autorest (aka `make generate-radclient`)
* [#890](https://github.com/Azure/radius/issues/890) To generate model objects to deserialized JSON from the Resource Provider side
* [#888](https://github.com/Azure/radius/issues/888) To generate model objects to be persisted in our Database]
* [#891](https://github.com/Azure/radius/issues/891) To generate documentation for our Resource Provider

## JSON Schema

### What is it?
JSON Schema is a vocabulary that allows you to annotate and validate JSON documents.

### How do we use it?

Take a look at `/pkg/radrp/schemas`. Our plan is:
* To validate JSON seen by the RP
* [#597](https://github.com/Azure/radius/issues/597) To validate JSON in the K8s webhook
* [#886](https://github.com/Azure/radius/issues/886) To validate JSON in the rad-bicep compiler

# Similarity between Open API v2 and JSON Schema Draft 4

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

## Can we generate one spec from the other?

Yes, we plan to generate one spec from the other. Currently already we share most of the definitions, except for the polymophics types like declared in `traits.json`.
