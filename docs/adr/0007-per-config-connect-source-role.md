# Per-Config Connect source role

Apply creates IAM role `relay-connect-{prefix}-cdc` for Debezium. Iceberg keeps the org Cluster Connect role. `rds-db:connect` for this Config's CDC user is on the source role only. A shared Connect role that accumulated every database login was rejected: any worker or plugin on the Cluster could open every Database.

Secrets Manager `GetSecretValue` on a CDC secret is superseded by 0010.
