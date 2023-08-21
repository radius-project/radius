output "result" {
  value = {
    values = {
      host = "test-host"
      port = 1234
    }
    secrets = {
      connectionString = "test-connectionString"
    }
    sensitive = true
  }
}