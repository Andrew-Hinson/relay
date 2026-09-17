terraform {
  required_version = ">= 1.10.0"

  backend "s3" {}

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kafka = {
      source  = "Mongey/kafka"
      version = "~> 0.8"
    }
    awscc = {
      source  = "hashicorp/awscc"
      version = "~> 1.0"
    }
  }
}

provider "aws" {
  region = var.region
}

provider "awscc" {
  region = var.region
}

provider "kafka" {
  bootstrap_servers = split(",", var.msk_bootstrap_servers)
  tls_enabled       = true
  sasl_mechanism    = "aws-iam"
  sasl_aws_region   = var.region
}
