variable "region" {
  type    = string
  default = "us-east-1"
}

variable "cluster" {
  type    = string
  default = "prod"
}

variable "instance_name" {
  type    = string
  default = "example-service"
}

variable "instance_create" {
  type    = bool
  default = false
}

variable "database_name" {
  type    = string
  default = "example-service"
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

variable "connect_subnet_ids" {
  type    = list(string)
  default = ["subnet-00000000"]
}

variable "connect_sg_ids" {
  type    = list(string)
  default = ["sg-00000000"]
}

variable "vpc_id" {
  type    = string
  default = "vpc-00000000"
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

variable "secret_name" {
  type    = string
  default = "example-service"
}

variable "secret_user_key" {
  type    = string
  default = "user"
}

variable "master_secret_arn" {
  type    = string
  default = ""
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
  default = "example-service-example-service-cdc"
}

variable "connector_class" {
  type    = string
  default = "io.debezium.connector.postgresql.PostgresConnector"
}

variable "connector_database" {
  type    = string
  default = "example-service"
}

variable "table_include_list" {
  type    = string
  default = "public.orders"
}

variable "connector_topic_prefix" {
  type    = string
  default = "example-service"
}

variable "publication_name" {
  type    = string
  default = "example_service_example_service_cdc"
}

variable "sink_name" {
  type    = string
  default = "example-service-exampleservicedb-iceberg"
}

variable "sink_control_topic" {
  type    = string
  default = "example-service.control.iceberg"
}

variable "connector_hostname" {
  type    = string
  default = "localhost"
}

variable "sink_topics" {
  type    = list(string)
  default = ["example-service.public.orders"]
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
    topic_name    = "example-service.public.orders"
    iceberg_table = "example_service_public_orders"
    route_value   = "public.orders"
    id_columns    = "id"
    columns = [
      { name = "id", type = "int" },
      { name = "amount", type = "decimal(38,9)" },
    ]
  }]
}
