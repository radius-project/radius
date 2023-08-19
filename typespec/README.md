# TypeSpec for Radius

TypeSpec is a language for describing cloud service APIs and generating other API description languages, client and service code, documentation, and other assets. TypeSpec provides highly extensible core language primitives that can describe API shapes common among REST, GraphQL, gRPC, and other protocols.

## Strcutures

* **[Applications.Link](./Applications.Link/)**: This directory contains typespec definitions for Applications.Link namespace.
* **[radius/v1](./radius/v1/)**: This directory contains the shared typespec libraries used by each namespace.

## Prerequisite

1. Install [NodeJS 16+](https://nodejs.org/en/download)
1. Install [TypeSpec compiler](https://microsoft.github.io/typespec/introduction/installation)
    ```bash
    npm install -g @typespec/compiler
    ```

## Build TypeSpec to OpenAPI swagger.

Radius uses [OpenAPI2 specifications](../swagger/) for defining API and validating the API request. You can compile and emit swagger spec files by following steps.

1. Install dependencies
   ```bash
   tsp install
   ```
1. Compile and emit the swagger files
   ```bash
   tsp compile ./Applications.Link
   ```

## References

* [Introduction to TypeSpec](https://microsoft.github.io/typespec/)
* [TypeSpec Azure](https://azure.github.io/typespec-azure/)
