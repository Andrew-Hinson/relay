# Per-Config Connect source role

Apply creates IAM role `relay-connect-{prefix}-cdc` for Debezium. Iceberg keeps the org Cluster Connect role. Secrets Manager `GetSecretValue` for this Config role secret is on the source role only. A shared Connect role that accumulated every RDS master secret was rejected: any worker or plugin on the Cluster could read every Relay-created password.
