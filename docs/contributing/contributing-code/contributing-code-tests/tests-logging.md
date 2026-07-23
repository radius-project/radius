# Radius test contexts and logging

Use `t.Context()` when code under test needs a context scoped to the test lifetime. The context is canceled before test cleanup functions run. Use `t.Log` or `t.Logf` for test diagnostics.

```go
import (
    "context"
    "testing"
    "time"
)

// Test_Render_Simple uses a context scoped to the test.
func Test_Render_Simple(t *testing.T) {
    ctx := t.Context()

    resources, err := renderer.Render(ctx, nil)
    // ...
}

// Test_Render_WithCancel can cancel the work before the test completes.
func Test_Render_WithCancel(t *testing.T) {
    ctx, cancel := context.WithCancel(t.Context())
    defer cancel()

    resources, err := renderer.Render(ctx, nil)
    // ...
}

// Test_Render_WithTimeout limits how long the work can run.
func Test_Render_WithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
    defer cancel()

    resources, err := renderer.Render(ctx, nil)
    // ...
}
```
