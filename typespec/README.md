# TypeSpec for Radius

TypeSpec is a language for describing cloud service APIs and generating other API description languages, client and service code, documentation, and other assets. TypeSpec provides highly extensible core language primitives that can describe API shapes common among REST, GraphQL, gRPC, and other protocols.

## Directory structure

### TypeSpec

* **[Applications.Core](./Applications.Core/)**: This directory contains Applications.Core TypeSpec files to define its namespace and resource types.
* **[Test.Resource](./Test.Resource/)**: This directory contains the template typespec files to create new namespace and resource types.
* **[radius/v1](./radius/v1/)**: This directory contains the radius shared typespec v1 libraries used by each namespace.

### OpenAPIv2 Spec file output

Once you compile your typespec files, the default output OpenAPIv2 spec file will be emitted to [/swagger/specification/applications/resource-manager](../swagger/specification/swagger/specification/applications/resource-manager).

## Prerequisite

1. Install [NodeJS 16+](https://nodejs.org/en/download)
1. Install [TypeSpec compiler](https://microsoft.github.io/typespec/introduction/installation)
    ```bash
    npm install -g @typespec/compiler
    ```

## Build TypeSpec to OpenAPI swagger.

Radius uses [OpenAPIv2 specifications](../swagger/) for defining API and validating the API request. You can compile and emit swagger spec files by following steps.

1. Install dependencies
   ```bash
   tsp install
   ```
1. Compile and emit the swagger files
   ```bash
   tsp compile ./Test.Resource
   ```
   Please ensure that you resolve all warnings and errors from the compiler.
1. Review your emitted swagger file under [/swagger/specification/applications/resource-manager/Test.Resource](../swagger/specification/applications/resource-manager/Test.Resource).


## TypeSpec authoring guideline

This section provides the tips and guidelines to define APIs with TypeSpec.

### TypeSpec file formatting

TypeSpec compiler has its own formatting TypeSpec files. Ensure that you run the following command once you edit spec files.

```bash
tsp format **/*.tsp
```

### Use [Test.Resource](./Test.Resource/) template to create new namespace

1. Copy the entire [Test.Resource](./Test.Resource/) directory to new directory with the new namespace name under [typespec](./).
1. Open [main.tsp](./Test.Resource/main.tsp) and update `Versions` enum to support new API version for your namespace.
1. Create new `ResourceTypeName.tsp` file to define new resource type based on [testasyncresources.tsp](./Test.Resource/testasyncresources.tsp) or [testsyncresources.tsp](./Test.Resource/testsyncresources.tsp).
1. Add `import "ResourceTypeName.tsp";` in `main.tsp` and remove the sample resource type tsp imports.
1. Run the formatter and compiler
   ```bash
   tsp format **/*.tsp
   tsp compile ./YourNamespace
   ```

### Support multiple API versions

You can manage multiple API versions with the decorator of [TypeSpec.Versioning](https://microsoft.github.io/typespec/standard-library/versioning/reference) library.

1. Describe the supported version in `enum Verisons` of `main.tsp`. For example, [Test.Resource/main.tsp](./Test.Resource/main.tsp) supports two API verisons, `2022-08-19-preview` and `2023-08-19`.
1. Use [the versioning decorators](https://microsoft.github.io/typespec/standard-library/versioning/reference#decorators) for model and property. [Test.Resource/typesyncresources.tsp](./Test.Resource/testsyncresources.tsp) includes the example to use `@added` decorator to add new resource type in `v2023-08-19` API version.

### Link API operation to example files with `x-ms-examples` custom property

With TypeSpec, we do not need to specify the properties for `x-ms-examples`. Instead, TypeSpec emitter library has the built-in feature to link resource operations to request/response example files automatically. To leverage this feature, JSON example files needs to be located under 
`/typespec/<ResourceNamespace>/examples/<API-version>/`. `operationId` property value in example file must match `<interface name>_<operation name>`.

For example, [TestSyncResource](./Test.Resource/testsyncresources.tsp) defines the following operations:

```ts
@added(Versions.v2023_08_19)
@armResourceOperations
interface TestSyncResources {
  get is ArmResourceRead<TestSyncResource, UCPBaseParameters<TestSyncResource>>;

  createOrUpdate is ArmResourceCreateOrReplaceSync<
    TestSyncResource,
    UCPBaseParameters<TestSyncResource>
  >;
  ...
```

You can create [TestSyncResource_Get.json](./Test.Resource/examples/2023-08-19/TestSyncResource_Get.json) and [TestSyncResource_CreateOrUpdate.json](./Test.Resource/examples/2023-08-19/TestSyncResource_CreateOrUpdate.json) in `Test.Resource/examples/2023-08-19/` like the following sample.

```json
{
  "operationId": "TestSyncResources_Get", // <-- This must match the name convention - "<interface name>_<operation name>".
  "title": "Get a TestSyncResources resource",
  "parameters": {
    "rootScope": "/planes/radius/local/resourceGroups/testGroup",
    "testSyncResourceName": "resource0",
    "api-version": "2023-08-19"
  },
  // ...
```

## References

* [Introduction to TypeSpec](https://microsoft.github.io/typespec/)
* [TypeSpec Azure](https://azure.github.io/typespec-azure/)
* [TypeSpec Samples](https://github.com/microsoft/typespec/tree/main/packages/samples)
* [TypeSpec Azure samples](https://github.com/Azure/typespec-azure/tree/main/packages/samples/specs/resource-manager)