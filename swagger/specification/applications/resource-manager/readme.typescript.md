## TypeScript

These settings apply only when `--typescript` is specified on the command line.
Please also specify `--typescript-sdks-folder=<path to root folder of your azure-sdk-for-js clone>`.

``` yaml $(typescript)
typescript:
  azure-arm: true
  batch: true
  payload-flattening-threshold: 1
  clear-output-folder: true
  generate-metadata: true
batch:
  - package-core: true
  - package-link: true
```

```yaml $(typescript) && $(package-core)
typescript:
  package-name: "@azure/arm-applications-core"
  output-folder: "$(typescript-sdks-folder)/sdk/applications/arm-applications-core"
  clear-output-folder: true
```

```yaml $(typescript) && $(package-link)
typescript:
  package-name: "@azure/arm-applications-link"
  output-folder: "$(typescript-sdks-folder)/sdk/applications/arm-applications-link"
  clear-output-folder: true
```