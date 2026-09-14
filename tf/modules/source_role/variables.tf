variable "prefix" {
  type = string
}

variable "connector_name" {
  type = string
}

variable "cluster_arn" {
  type = string
}

variable "secret_arn" {
  type    = string
  default = ""
}

variable "permissions_boundary_arn" {
  type = string
}

variable "worker_policy_arn" {
  type = string
}
