variable "name" {
  description = "The name to use for the Kubernetes resources. Radius resolves this from a {{context.resource.name}} expression."
  type        = string
}

variable "namespace" {
  description = "The namespace to deploy into. Radius resolves this from a {{context.runtime.kubernetes.namespace}} expression."
  type        = string
}

variable "port" {
  description = "The container port to expose."
  type        = number
  default     = 6379
}
