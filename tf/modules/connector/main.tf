resource "aws_mskconnect_worker_configuration" "this" {
  name                    = "${var.name}-worker"
  properties_file_content = <<-EOT
  key.converter=org.apache.kafka.connect.storage.StringConverter
  value.converter=org.apache.kafka.connect.storage.StringConverter
  EOT
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

  connector_configuration = {
    "connector.class"                           = var.class
    "tasks.max"                                 = "1"
    "database.hostname"                         = var.database_hostname
    "database.port"                             = "5432"
    "database.user"                             = var.cdc_user
    "database.dbname"                           = var.database_name
    "database.sslmode"                          = "require"
    "database.connection.factory.class"         = "io.debezium.connector.postgresql.connection.PostgresAwsIamConnectionFactory"
    "topic.prefix"                              = var.topic_prefix
    "table.include.list"                        = var.table_include_list
    "plugin.name"                               = "pgoutput"
    "slot.name"                                 = replace(var.name, "-", "_")
    "publication.name"                          = var.publication_name
    "publication.autocreate.mode"               = "disabled"
    "key.converter"                             = "org.apache.kafka.connect.json.JsonConverter"
    "value.converter"                           = "org.apache.kafka.connect.json.JsonConverter"
    "key.converter.schemas.enable"              = "true"
    "value.converter.schemas.enable"            = "true"
    "topic.creation.enable"                     = "true"
    "topic.creation.default.partitions"         = tostring(var.partitions)
    "topic.creation.default.replication.factor" = tostring(var.replicas)
    "config.action.reload"                      = "restart"
  }

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

  worker_configuration {
    arn      = aws_mskconnect_worker_configuration.this.arn
    revision = aws_mskconnect_worker_configuration.this.latest_revision
  }
}
