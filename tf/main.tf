module "rds" {
  count  = var.instance_create ? 1 : 0
  source = "./modules/rds"

  name            = var.instance_name
  username        = var.rds_username
  password        = var.master_password
  engine_version  = var.rds_engine_version
  instance_class  = var.rds_instance_class
  subnet_ids      = var.subnet_ids
  security_groups = var.rds_sg_ids
}

module "topic" {
  for_each = { for t in var.tables : t.topic_name => t }

  source = "./modules/topic"

  name                = each.value.topic_name
  partitions          = var.partitions
  replicas            = var.replicas
  min_insync_replicas = var.min_insync_replicas
}

module "acl" {
  source = "./modules/acl"

  role_arn    = var.connect_role_arn
  cluster_arn = var.msk_cluster_arn
  prefix      = var.connector_topic_prefix
}

module "connector" {
  source = "./modules/connector"

  name               = var.connector_name
  class              = var.connector_class
  plugin_arn         = var.debezium_plugin_arn
  role_arn           = var.connect_role_arn
  bootstrap_servers  = var.msk_bootstrap_servers
  subnet_ids         = var.connect_subnet_ids
  security_groups    = var.connect_sg_ids
  database_hostname  = var.connector_hostname
  database_name      = var.connector_database
  table_include_list = var.table_include_list
  topic_prefix       = var.connector_topic_prefix
  publication_name   = var.publication_name
  secret_name        = var.instance_name
  partitions         = var.partitions
  replicas           = var.replicas
}

module "sink" {
  source = "./modules/sink"

  name              = var.sink_name
  plugin_arn        = var.iceberg_plugin_arn
  role_arn          = var.connect_role_arn
  bootstrap_servers = var.msk_bootstrap_servers
  subnet_ids        = var.connect_subnet_ids
  security_groups   = var.connect_sg_ids
  topics            = var.sink_topics
  warehouse_bucket  = var.warehouse_bucket
  glue_database     = var.glue_database
  tables            = var.tables
}
