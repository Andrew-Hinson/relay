output "endpoint" {
  value = awscc_rds_db_instance.this.endpoint.address
}

output "resource_id" {
  value = awscc_rds_db_instance.this.dbi_resource_id
}
