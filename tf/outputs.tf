output "rds_endpoint" {
  value = try(module.rds[0].endpoint, "")
}

output "rds_resource_id" {
  value = try(module.rds[0].resource_id, "")
}
