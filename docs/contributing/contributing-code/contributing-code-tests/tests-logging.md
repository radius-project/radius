# Radius tests logging

The Radius tests redirect the Resource Provider logger output to the testing error log. This can be done as below:-

```go
import (
    ...
    "github.com/project-radius/radius/test/testcontext"
    ...
)

// Test_Render_Simple uses the default logger context.
func Test_Render_Simple(t *testing.T) {
    ctx := testcontext.New(t)

    ...
    resources, err := renderer.Render(ctx, nil)
    ...
}

// Test_Render_WithCancel uses the default logger context with context cancel function.
func Test_Render_WithCancel(t *testing.T) {
    ctx, cancel := testcontext.NewWithCancel(t)
    t.Cleanup(cancel)

    ...
    resources, err := renderer.Render(ctx, nil)
    ...
}

```
