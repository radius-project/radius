extension mycompanytest

resource widget 'MyCompany.Test/widgets@2025-01-01-preview' = {
  name: 'my-widget'
  properties: {
    message: 'hello from radius'
    count: 3
    enabled: true
  }
}

// Backward-compatible nested envelope form must continue to work.
output widgetMessage string = widget.properties.message
