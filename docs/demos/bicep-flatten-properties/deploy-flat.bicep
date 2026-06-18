extension mycompanytest

resource widget 'MyCompany.Test/widgets@2025-01-01-preview' = {
  name: 'my-widget'
  properties: {
    message: 'hello from radius'
    count: 3
    enabled: true
  }
}

// Flat ReadOnly aliases introduced by PR #12132 — read without `.properties.`
output widgetMessage string = widget.message
output widgetCount int = widget.count
output widgetEnabled bool = widget.enabled
