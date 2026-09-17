resource "aws_db_subnet_group" "this" {
  name       = var.name
  subnet_ids = var.subnet_ids
}

resource "aws_db_parameter_group" "this" {
  name   = var.name
  family = "postgres${split(".", var.engine_version)[0]}"

  parameter {
    name         = "rds.logical_replication"
    value        = "1"
    apply_method = "pending-reboot"
  }

  parameter {
    name         = "rds.iam_auth_for_replication"
    value        = "1"
    apply_method = "immediate"
  }
}

resource "awscc_rds_db_instance" "this" {
  db_instance_identifier             = var.name
  engine                             = "postgres"
  engine_version                     = var.engine_version
  db_instance_class                  = var.instance_class
  allocated_storage                  = "20"
  master_username                    = var.username
  master_user_authentication_type    = "iam-db-auth"
  enable_iam_database_authentication = true
  db_subnet_group_name               = aws_db_subnet_group.this.name
  db_parameter_group_name            = aws_db_parameter_group.this.name
  vpc_security_groups                = var.security_groups
  storage_encrypted                  = true
  deletion_protection                = true
  apply_immediately                  = true
}
