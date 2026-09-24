locals {
  topic_arn = "${replace(var.cluster_arn, ":cluster/", ":topic/")}/${var.prefix}.*"
  group_arn = replace(var.cluster_arn, ":cluster/", ":group/")
  role_name = element(split("/", var.role_arn), length(split("/", var.role_arn)) - 1)
}

data "aws_iam_policy_document" "connect" {
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
    resources = ["${local.group_arn}/${var.prefix}-*", "${local.group_arn}/connect-${var.prefix}-*"]
  }
}

resource "aws_iam_role_policy" "connect" {
  name   = "${replace(var.prefix, ".", "-")}-topics"
  role   = local.role_name
  policy = data.aws_iam_policy_document.connect.json
}
