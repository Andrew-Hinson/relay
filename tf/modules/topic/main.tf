resource "kafka_topic" "this" {
  name               = var.name
  replication_factor = var.replicas
  partitions         = var.partitions

  config = {
    "min.insync.replicas" = tostring(var.min_insync_replicas)
    "cleanup.policy"      = var.cleanup_policy
  }

  # Dropping or replacing a topic loses CDC history. Removing a Table from a Config
  # needs a deliberate `terraform state rm` first.
  lifecycle {
    prevent_destroy = true
  }
}
