output "rds_endpoint" {
  value = try(module.rds[0].endpoint, "")
}

output "rds_master_secret_arn" {
  value = try(module.rds[0].master_secret_arn, "")
}
