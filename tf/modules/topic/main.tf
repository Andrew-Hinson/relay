resource "kafka_topic" "this" {
  name               = var.name
  replication_factor = var.replicas
  partitions         = var.partitions

  config = {
    "min.insync.replicas" = tostring(var.min_insync_replicas)
    "cleanup.policy"      = var.cleanup_policy
  }
}
