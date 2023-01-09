# Radius resource provider logging

The Radius RP logs will be used for debugging and troubleshooting Radius deployments and end-to-end deployment tests. These logs should therefore provide meaningful information and context which would be needed for troubleshooting.

## Log Context Passing

When the RP is created, a new log context is created and information such as the subscription ID, resource group is added to it. This log context is then passed around and every entry point can add more information to the same log context as it becomes available e.g. applicationName, resourceID, etc.

## Logging Best Practices

* If a new entry point is introduced, make sure it accepts a context and pass in the main context with the logger. For example:

```go
func (r *rp) UpdateDeployment(**ctx context.Context**, d *rest.Deployment) (rest.Response, error) {
    ....
}
```

* Inside a function, create a logger from the input context to log messages.

```go
logger := logr.FromContextOrDiscard(ctx)
```

* Whenever there is more new relevant information that becomes available in a method, add new information fields to the logs. Radius uses a structured format for logging. Add a new constant field under the ucplogger package and add it to the logging context.

```go
const (
LogFieldAppName            = "applicationName"
    ...
)

ctx = ucplog.WrapLogContext(ctx,
    logging.LogFieldAppName, id.App.Name(),
    logging.LogFieldAppID, id.App.ID)
logger := logr.FromContextOrDiscard(ctx)
```
