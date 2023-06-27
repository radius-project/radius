# Radius tests logging

The Radius tests redirect the Resource Provider logger output to the testing error log. This can be done as below:-

```go
import (
    ...
    "github.com/project-radius/radius/test/testcontext"
    ...
)

func Test_Render_Simple(t *testing.T) {
    ctx, cancel := testcontext.NewContext(t, nil)
    defer cancel()

    ...
    resources, err := renderer.Render(ctx, nil)
    ...
}
```
