module "rds" {
  count  = var.instance_create ? 1 : 0
  source = "./modules/rds"

  name            = var.instance_name
  username        = var.rds_username
  engine_version  = var.rds_engine_version
  instance_class  = var.rds_instance_class
  subnet_ids      = var.subnet_ids
  security_groups = var.rds_sg_ids
}

data "aws_db_instance" "attached" {
  count                  = var.instance_create ? 0 : 1
  db_instance_identifier = var.instance_name
}

locals {
  instance_resource_id = var.instance_create ? module.rds[0].resource_id : data.aws_db_instance.attached[0].resource_id
}

module "topic" {
  for_each = { for t in var.tables : t.topic_name => t }

  source = "./modules/topic"

  name                = each.value.topic_name
  partitions          = var.partitions
  replicas            = var.replicas
  min_insync_replicas = var.min_insync_replicas
}

module "control_topic" {
  source = "./modules/topic"

  name                = var.sink_control_topic
  partitions          = 1
  replicas            = var.replicas
  min_insync_replicas = var.min_insync_replicas
  cleanup_policy      = "compact"
}

module "source_role" {
  source = "./modules/source_role"

  prefix                   = var.connector_topic_prefix
  connector_name           = var.connector_name
  cluster_arn              = var.msk_cluster_arn
  cdc_user                 = var.cdc_user
  instance_resource_id     = local.instance_resource_id
  permissions_boundary_arn = var.connect_source_boundary_arn
  worker_policy_arn        = var.connect_worker_policy_arn
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
  role_arn           = module.source_role.arn
  bootstrap_servers  = var.msk_bootstrap_servers
  subnet_ids         = var.connect_subnet_ids
  security_groups    = var.connect_sg_ids
  database_hostname  = var.connector_hostname
  database_name      = var.connector_database
  table_include_list = var.table_include_list
  topic_prefix       = var.connector_topic_prefix
  publication_name   = var.publication_name
  cdc_user           = var.cdc_user
  partitions         = var.partitions
  replicas           = var.replicas

  depends_on = [module.source_role]
}

module "sink" {
  source = "./modules/sink"

  name                 = var.sink_name
  plugin_arn           = var.iceberg_plugin_arn
  role_arn             = var.connect_role_arn
  bootstrap_servers    = var.msk_bootstrap_servers
  subnet_ids           = var.connect_subnet_ids
  security_groups      = var.connect_sg_ids
  topics               = var.sink_topics
  control_topic        = var.sink_control_topic
  control_group_prefix = var.connector_topic_prefix
  region               = var.region
  warehouse_bucket     = var.warehouse_bucket
  glue_database        = var.glue_database
  tables               = var.tables

  depends_on = [module.control_topic]
}
