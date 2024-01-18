// A simple Bicep recipe that tests context parameter is applied. It doesn't provision any resources.
param context object

output result object = {
  values: {
    environment: context.environment.Name
    application: context.application.Name
    resource: context.resource.Name
    namespace: context.runtime.kubernetes.namespace
    envNamespace: context.runtime.kubernetes.environmentNamespace
  }
}
