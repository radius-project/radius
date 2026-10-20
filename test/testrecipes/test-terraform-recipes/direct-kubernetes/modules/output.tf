output "host" {
  description = "The in-cluster DNS name of the Redis service. Mapped onto a resource property via the recipe `outputs` field."
  value       = "${kubernetes_service.redis.metadata[0].name}.${kubernetes_service.redis.metadata[0].namespace}.svc.cluster.local"
}

output "port" {
  description = "The port exposed by the Redis service."
  value       = tostring(var.port)
}
