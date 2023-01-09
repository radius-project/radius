# Radius tests logging

The Radius tests redirect the Resource Provider logger output to the testing error log. This can be done as below:-

```go
func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Simple(t *testing.T) {
	ctx := createContext(t)
    .....
    resources, err := renderer.Render(ctx, w)
    ....
}
```
