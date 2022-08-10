# Cadl-fying Radius
Cadl is a modeling language based on typescript that can be used to generate OpenAPI and swagger specs. Follow the tutorial to install and get the environment set up.

## Important Resources
- [Cadl Repository](https://github.com/microsoft/cadl "Cadl Repository")
- [Cadl Tutorial](https://github.com/microsoft/cadl/blob/main/docs/tutorial.md)
- [Cadl for the OpenAPI developer](https://github.com/microsoft/cadl/blob/34eaea96bb2e355d4df5bed0b3a1eeeee34a03bf/docs/cadl-for-openapi-dev.md)
- [Cadl Azure Playground](https://cadlplayground.z22.web.core.windows.net/cadl-azure/ "Cadl Azure Playground")
- [Cadl Discussion Teams Channel](https://teams.microsoft.com/l/channel/19%3a906c1efbbec54dc8949ac736633e6bdf%40thread.skype/Cadl%2520Discussion%2520%25F0%259F%2590%25AE?groupId=3e17dcb0-4257-4a30-b843-77f47f1d4121&tenantId=72f988bf-86f1-41af-91ab-2d7cd011db47) (Note: You may need to ask someone for access)

## Recommended Dependencies
- @azure-tools/cadl-autorest
- @azure-tools/cadl-azure-core
- @azure-tools/cadl-azure-resource-manager
- @azure-tools/cadl-providerhub
- @cadl-lang/compiler
- @cadl-lang/openapi3
- @cadl-lang/rest

## Tracked Resources
Currently all of our resources are tracked resources. That means that when writing a new resource, each file will have the following:

```TypeScript
model ResourceProperties {}

model Resource is TrackedResource<ResourceProperties> {
    name: string;
}

@armResourceOperations
interface InterfaceName 
    extends Radius.RootScopeResourceOperations<ResourceName, ResourceProperties, RootScopeParam>
```
There may be more or less depending on the  resource being modeled

## {rootScope}
At the time of writing this, the Radius team's spec has not been approved by ARM. As a result, the Cadl team has created a custom `RootScopeResourceOperations` object for us to use.

To utilize this, we need to do the following:
1. Import `"./customRootScope.cadl` into our resource file.
2. When creating the `@armResourceOperations` we use the `RootScopeResourceOperations` object under the Radius namespace instead of the standard `ResourceOperations` object:
```TypeScript
@armResourceOperations
interface InterfaceName 
	extends Radius.RootScopeResourceOperations<Resource, ResourceProperties, RootScopeParam>
```

## Emitting and Compiling
In the `cadl-project.yaml` the emitter is set to `"@azure-tools/cadl-autorest": true`. This means that we compile to swagger instead of OpenApi3. If you want to compile to OpenApi3, set the emitter to `"@cadl-lang/openapi3": true`.

To compile with {rootScope} to a custom file, run the following command in the terminal:
```TypeScript
cadl compile {fileName}.cadl --import "./aksrootscope.cadl" --option "@azure-tools/cadlautorest.output-file={fileName}.json"
```

To compile with the ARM compliant spec to a custom file, run the following command in the terminal:
```TypeScript
cadl compile {fileName}.cadl --import "./armrootscope.cadl" --option "@azure-tools/cadlautorest.output-file={fileName}.json"
```

In both cases replace {fileName} with the file you want to compile.

## Formatting
Run `cadl compile .` to format all files.
