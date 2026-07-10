variable "context" {
  description = "This variable contains Radius Recipe context."
  type        = any
  default     = null
}

variable "skuName" {
  description = "The SKU name for the MySQL Flexible Server (e.g. B_Standard_B1ms, GP_Standard_D2ds_v4)."
  type        = string
  default     = "B_Standard_B1ms"
}

variable "storageSizeGb" {
  description = "Storage size in GB for the MySQL server."
  type        = number
  default     = 20
}
