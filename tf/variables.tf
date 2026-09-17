variable "region" {
  type    = string
  default = "us-east-1"
}

variable "instance_name" {
  type    = string
  default = "example"
}

variable "instance_create" {
  type    = bool
  default = false
}

variable "database_name" {
  type    = string
  default = "exampledb"
}

variable "msk_bootstrap_servers" {
  type    = string
  default = "localhost:9092"
}

variable "msk_cluster_arn" {
  type    = string
  default = "arn:aws:kafka:us-east-1:000000000000:cluster/prod/00000000-0000-0000-0000-000000000000-0"
}

variable "warehouse_bucket" {
  type    = string
  default = "relay-warehouse"
}

variable "glue_database" {
  type    = string
  default = "relay"
}

variable "debezium_plugin_arn" {
  type    = string
  default = "arn:aws:kafkaconnect:us-east-1:000000000000:custom-plugin/debezium/00000000-0000-0000-0000-000000000000-1"
}

variable "iceberg_plugin_arn" {
  type    = string
  default = "arn:aws:kafkaconnect:us-east-1:000000000000:custom-plugin/iceberg/00000000-0000-0000-0000-000000000000-1"
}

variable "connect_role_arn" {
  type    = string
  default = "arn:aws:iam::000000000000:role/relay-connect"
}

variable "connect_source_boundary_arn" {
  type    = string
  default = "arn:aws:iam::000000000000:policy/relay-connect-source-boundary"
}

variable "connect_worker_policy_arn" {
  type    = string
  default = "arn:aws:iam::000000000000:policy/relay-connect-worker"
}

variable "connect_subnet_ids" {
  type    = list(string)
  default = ["subnet-00000000"]
}

variable "connect_sg_ids" {
  type    = list(string)
  default = ["sg-00000000"]
}

variable "subnet_ids" {
  type    = list(string)
  default = ["subnet-00000000"]
}

variable "rds_sg_ids" {
  type    = list(string)
  default = ["sg-00000001"]
}

variable "rds_instance_class" {
  type    = string
  default = "db.t3.medium"
}

variable "rds_engine_version" {
  type    = string
  default = "16"
}

variable "rds_username" {
  type    = string
  default = "relay"
}

variable "cdc_user" {
  type    = string
  default = "example_exampledb_cdc"
}

variable "partitions" {
  type    = number
  default = 3
}

variable "replicas" {
  type    = number
  default = 3
}

variable "min_insync_replicas" {
  type    = number
  default = 2
}

variable "connector_name" {
  type    = string
  default = "example-exampledb-cdc"
}

variable "connector_class" {
  type    = string
  default = "io.debezium.connector.postgresql.PostgresConnector"
}

variable "connector_database" {
  type    = string
  default = "exampledb"
}

variable "table_include_list" {
  type    = string
  default = "public.orders"
}

variable "connector_topic_prefix" {
  type    = string
  default = "example"
}

variable "publication_name" {
  type    = string
  default = "example_exampledb_cdc"
}

variable "sink_name" {
  type    = string
  default = "example-exampledb-iceberg"
}

variable "sink_control_topic" {
  type    = string
  default = "example.control.iceberg"
}

variable "connector_hostname" {
  type    = string
  default = "localhost"
}

variable "sink_topics" {
  type    = list(string)
  default = ["example.public.orders"]
}

variable "tables" {
  type = list(object({
    topic_name    = string
    iceberg_table = string
    route_value   = string
    id_columns    = string
    columns = list(object({
      name = string
      type = string
    }))
  }))
  default = [{
    topic_name    = "example.public.orders"
    iceberg_table = "example_public_orders"
    route_value   = "public.orders"
    id_columns    = "id"
    columns = [
      { name = "id", type = "int" },
      { name = "amount", type = "decimal(38,9)" },
    ]
  }]
}
