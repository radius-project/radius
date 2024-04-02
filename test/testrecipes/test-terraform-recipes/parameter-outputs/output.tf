output "result" {
  value = {
    values = {
      a = var.a
      b = var.b
      c = var.c
      d = var.d
    }
    secrets = {
      e = "secret value"
    }
  }
}