# TypeSpec for Radius

TypeSpec is a language for describing cloud service APIs and generating other API description languages, client and service code, documentation, and other assets. TypeSpec provides highly extensible core language primitives that can describe API shapes common among REST, GraphQL, gRPC, and other protocols.

## Strcutures

* **[Applications.Core](./Applications.Core/): This directory contains Applications.Core TypeSpec files to define its namespace and resource types.
* **[Test.Resource](./Test.Resource/): This directory contains the template typespec files to create new namespace and resource types.
* **[radius/v1](./radius/v1/)**: This directory contains the radius shared typespec v1 libraries used by each namespace.

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

## Formatting

TypeSpec compiler has its own formatting TypeSpec files. Ensure that you run the following command once you edit spec files.

```
tsp format **/*.tsp
```

## References

* [Introduction to TypeSpec](https://microsoft.github.io/typespec/)
* [TypeSpec Azure](https://azure.github.io/typespec-azure/)
* [TypeSpec Samples](https://github.com/microsoft/typespec/tree/main/packages/samples)
* [TypeSpec Azure samples](https://github.com/Azure/typespec-azure/tree/main/packages/samples/specs/resource-manager)