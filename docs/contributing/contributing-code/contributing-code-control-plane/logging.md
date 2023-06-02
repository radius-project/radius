# Radius control-plane logging

Logs can be used for debugging and troubleshooting Radius deployments and end-to-end deployment tests. These logs should therefore provide meaningful information and context which would be needed for troubleshooting.

In the control-plane we use the [logr](https://github.com/go-logr/logr) interface and [zap](https://github.com/uber-go/zap) as the underlying implementation. Logging statements in the control-plane components MUST use these library and not other APIs like `fmt.Printf` or the built-in `"log"` package. 

> ⚡️⚡️⚡️ Note ⚡️⚡️⚡️
>
> We do user-visible output differently in the CLI and don't use either `logr` or `zap` in that part of the code.

## Log Context Passing

 Every HTTP request to the API or work item for the backend will create its own log context with the relevant information (HTTP request URL, resource id, etc). The context is then passed through the call stack. Creating a new context can add more information as it becomes available.

## Logging Best Practices

* If a new entry major component is introduced, make sure it accepts a context. For example:

```go
func (r *rp) UpdateDeployment(ctx context.Context, d *rest.Deployment) (rest.Response, error) {
    ....
}
```

* If calling a function that accepts a context, make sure to provide the one passed to your function (NOT `context.TODO` or `context.Background`):

```go
func myFunc(ctx context.Context) {
    rp.DoSomething(ctx)
}
```

* Inside a function, create a logger from the input context to log messages.

```go
logger := ucplog.FromContextOrDiscard(ctx)
```

* Logging statements should begin with a message followed by optional key-value pairs. Optimize for both the message being readable and the key-value-pairs being searchable.

```go
logger.Info(fmt.Sprintf("processing cloud resource %s", resourceID), "targetResourceID", resourceID)
```

* Use `logger.Error` when logging errors. Only log errors when they are handled. Logging errors where they are simply propagated will result in the same failure being logged many times:

```go

// GOOD
err := rp.doThings(ctx)
if err != nil {
    // respond to user with HTTP 500
    logger.Err(e, "failed to do things")
    response.Write(500)
}

// BAD
err := rp.doDifferentThings(ctx)
if err != nil {
    logger.Err(e, "failed to do different things")
    return err
}
```

* If you want to add context about a particular type of failure, do this with error wrapping not by logging the error multiple types.

// GOOD
err := rp.doDifferentThings(ctx)
if err != nil {
    return fmt.Errorf("failed to different things: %w", err)
}
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
logger := ucplog.FromContextOrDiscard(ctx)
```

## When to log

