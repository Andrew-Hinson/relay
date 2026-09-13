variable "name" {
  type = string
}

variable "partitions" {
  type = number
}

variable "replicas" {
  type = number
}

variable "min_insync_replicas" {
  type = number
}

variable "cleanup_policy" {
  type    = string
  default = "delete"
}
