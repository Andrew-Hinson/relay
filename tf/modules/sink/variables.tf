variable "name" {
  type = string
}

variable "plugin_arn" {
  type = string
}

variable "plugin_revision" {
  type    = number
  default = 1
}

variable "role_arn" {
  type = string
}

variable "bootstrap_servers" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "security_groups" {
  type = list(string)
}

variable "topics" {
  type = list(string)
}

variable "control_topic" {
  type = string
}

variable "control_group_prefix" {
  type = string
}

variable "region" {
  type = string
}

variable "warehouse_bucket" {
  type = string
}

variable "glue_database" {
  type = string
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
}

variable "kafkaconnect_version" {
  type    = string
  default = "2.7.1"
}
