variable "password" {
  description = "The password for the PostgreSQL database"
  type        = string
}

variable "host" {
  default = "localhost"
}