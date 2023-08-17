variable "context" {
  description = "This variable contains Radius recipe context."
  type = object({
    resource = object({
      name = string
      id = string
      type = string
    })

    application = object({
      name = string
      id = string
    })

    environment = object({
      name = string
      id = string
    })

    runtime = object({
      kubernetes = optional(object({
        namespace = string
        environmentNamespace = string
      }))
    })

    azure = optional(object({
      resourceGroup = object({
        name = string
        id = string
      })
      subscription = object({
        subscriptionId = string
        id = string
      })
    }))
    
    aws = optional(object({
      region = string
      account = string
    }))
  })
}
