variable "message" {
  type = string
}

resource "terraform_data" "identity" {
  input = var.message
}

output "result" {
  value = {
    resources = []
    values = {
      identity = terraform_data.identity.id
      sentinel = "hash-upgrade-fixture"
    }
  }
}
