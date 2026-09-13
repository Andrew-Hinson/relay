locals {
  iceberg_tables = join(",", [for t in var.tables : "${var.glue_database}.${t.iceberg_table}"])
  table_config = merge([
    for t in var.tables : {
      "iceberg.table.${var.glue_database}.${t.iceberg_table}.id-columns"  = t.id_columns
      "iceberg.table.${var.glue_database}.${t.iceberg_table}.route-regex" = "^${replace(t.route_value, ".", "\\.")}$"
    }
  ]...)
}

resource "aws_glue_catalog_table" "iceberg" {
  for_each = { for t in var.tables : t.iceberg_table => t }

  name          = each.value.iceberg_table
  database_name = var.glue_database
  table_type    = "EXTERNAL_TABLE"

  parameters = {
    table_type = "ICEBERG"
  }

  storage_descriptor {
    location = "s3://${var.warehouse_bucket}/${var.glue_database}/${each.value.iceberg_table}"

    dynamic "columns" {
      for_each = each.value.columns
      content {
        name = columns.value.name
        type = columns.value.type
      }
    }
  }

  open_table_format_input {
    iceberg_input {
      metadata_operation = "CREATE"
    }
  }
}

resource "aws_mskconnect_connector" "this" {
  name                 = var.name
  kafkaconnect_version = var.kafkaconnect_version

  capacity {
    provisioned_capacity {
      mcu_count    = 1
      worker_count = 1
    }
  }

  connector_configuration = merge(local.table_config, {
    "connector.class"                      = "org.apache.iceberg.connect.IcebergSinkConnector"
    "tasks.max"                            = "1"
    "topics"                               = join(",", var.topics)
    "iceberg.tables"                       = local.iceberg_tables
    "iceberg.tables.auto-create-enabled"   = "false"
    "iceberg.tables.evolve-schema-enabled" = "false"
    "iceberg.tables.route-field"           = "_cdc.target"
    "iceberg.control.topic"                = var.control_topic
    "iceberg.control.group-id-prefix"      = "${var.control_group_prefix}-"
    "iceberg.catalog.catalog-impl"         = "org.apache.iceberg.aws.glue.GlueCatalog"
    "iceberg.catalog.io-impl"              = "org.apache.iceberg.aws.s3.S3FileIO"
    "iceberg.catalog.warehouse"            = "s3://${var.warehouse_bucket}/"
    "iceberg.catalog.client.region"        = var.region
    "iceberg.catalog.glue.skip-archive"    = "true"
    "transforms"                           = "unwrap"
    "transforms.unwrap.type"               = "org.apache.iceberg.connect.transforms.DebeziumTransform"
    "key.converter"                        = "org.apache.kafka.connect.json.JsonConverter"
    "value.converter"                      = "org.apache.kafka.connect.json.JsonConverter"
    "key.converter.schemas.enable"         = "true"
    "value.converter.schemas.enable"       = "true"
  })

  kafka_cluster {
    apache_kafka_cluster {
      bootstrap_servers = var.bootstrap_servers

      vpc {
        security_groups = var.security_groups
        subnets         = var.subnet_ids
      }
    }
  }

  kafka_cluster_client_authentication {
    authentication_type = "IAM"
  }

  kafka_cluster_encryption_in_transit {
    encryption_type = "TLS"
  }

  plugin {
    custom_plugin {
      arn      = var.plugin_arn
      revision = var.plugin_revision
    }
  }

  service_execution_role_arn = var.role_arn

  depends_on = [aws_glue_catalog_table.iceberg]
}
