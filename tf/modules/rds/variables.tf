variable "name" {
  type = string
}

variable "username" {
  type = string
}

variable "engine_version" {
  type = string
}

variable "instance_class" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "security_groups" {
  type = list(string)
}
