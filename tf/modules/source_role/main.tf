data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  role_name = "relay-connect-${replace(var.prefix, ".", "-")}-cdc"
  topic_arn = "${replace(var.cluster_arn, ":cluster/", ":topic/")}/${var.prefix}*"
  group_arn = replace(var.cluster_arn, ":cluster/", ":group/")
  db_user   = "arn:aws:rds-db:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:dbuser:${var.instance_resource_id}/${var.cdc_user}"
}

data "aws_iam_policy_document" "trust" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["kafkaconnect.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:kafkaconnect:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:connector/${var.connector_name}/*"]
    }
  }
}

resource "aws_iam_role" "this" {
  name                 = local.role_name
  permissions_boundary = var.permissions_boundary_arn
  assume_role_policy   = data.aws_iam_policy_document.trust.json
}

resource "aws_iam_role_policy_attachment" "worker" {
  role       = aws_iam_role.this.name
  policy_arn = var.worker_policy_arn
}

data "aws_iam_policy_document" "kafka" {
  statement {
    actions   = ["kafka-cluster:Connect", "kafka-cluster:DescribeCluster", "kafka-cluster:WriteDataIdempotently"]
    resources = [var.cluster_arn]
  }

  statement {
    actions = [
      "kafka-cluster:CreateTopic",
      "kafka-cluster:DescribeTopic",
      "kafka-cluster:WriteData",
      "kafka-cluster:ReadData",
    ]
    resources = [local.topic_arn]
  }

  statement {
    actions   = ["kafka-cluster:AlterGroup", "kafka-cluster:DescribeGroup"]
    resources = ["${local.group_arn}/${var.prefix}*", "${local.group_arn}/connect-${var.prefix}*"]
  }
}

resource "aws_iam_role_policy" "kafka" {
  name   = "${replace(var.prefix, ".", "-")}-topics"
  role   = aws_iam_role.this.id
  policy = data.aws_iam_policy_document.kafka.json
}

data "aws_iam_policy_document" "rds" {
  statement {
    actions   = ["rds-db:connect"]
    resources = [local.db_user]
  }
}

resource "aws_iam_role_policy" "rds" {
  name   = "${replace(var.prefix, ".", "-")}-rds"
  role   = aws_iam_role.this.id
  policy = data.aws_iam_policy_document.rds.json
}
