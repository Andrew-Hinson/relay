variable "name" {
  type = string
}

variable "class" {
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

variable "database_hostname" {
  type = string
}

variable "database_name" {
  type = string
}

variable "table_include_list" {
  type = string
}

variable "topic_prefix" {
  type = string
}

variable "publication_name" {
  type = string
}

variable "cdc_user" {
  type = string
}

variable "partitions" {
  type = number
}

variable "replicas" {
  type = number
}

variable "kafkaconnect_version" {
  type    = string
  default = "3.7.x"
}
