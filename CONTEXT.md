# Relay

A Config is the YAML a service team commits. Apply attaches that Config to an existing Cluster: an Instance, one Database, Tables, and a streaming path to Iceberg.

## Language

**Config**:
The YAML a service team commits. Input to Apply. Identifiers are `[a-z][a-z0-9_]*`. An Instance name used on create is `[a-z][a-z0-9]*`.
_Avoid_: Project, manifest, Relay file

**Apply**:
Reconciliation of a Config. Shows the plan and asks before changing anything (`--yes` skips). v1 has no destroy: topics and Iceberg tables refuse deletion.

**Plan**:
Dry-run of Apply. Prints create vs teardown from Config vs live. Teardown is Config membership (Table, topic, Iceberg table). Apply does not DROP Postgres.

**Cluster**:
A named shared MSK, Connect, and Warehouse. A Config names it and never creates it.
_Avoid_: kind, per-Config Kafka

**Warehouse**:
The shared S3 bucket and Glue Iceberg catalog for a Cluster.
_Avoid_: per-Config bucket, landing zone

**Iceberg table**:
The derived lake relation for a Table. Name is Glue-safe `[a-z0-9_]` (`example` + `public.orders` → `example_public_orders`). BI reads it. Not a Relay object.

**Instance**:
The RDS server a Config's Database lives on. A Config creates it or names an existing one. Create names are `[a-z][a-z0-9]*`. Attach may use hyphens.
_Avoid_: cluster, RDS as the generic term

**Database**:
A named Postgres database on an Instance. This Config always creates it. One per Config. Share happens at Instance only. Names are `[a-z][a-z0-9_]*`. Default is the Config name plus a `db` suffix (`example` → `exampledb`).
_Avoid_: schema, database.create false

**Schema**:
A Postgres namespace inside a Database. Default `public`.

**Table**:
A named relation this Config owns. Columns are DDL. A primary key is required. The app writes rows only.

**Prefix**:
A Config override for resource names, defaulting to the Config name. Topics use `{prefix}.{schema}.{table}`.

**Owner role**:
Postgres LOGIN role Apply creates for this Config (`{prefix}_{database}`). Owns the Database and Tables. Isolated to this Config's Database (`REVOKE CONNECT FROM PUBLIC`). Has `rds_iam`. The printed connection user. Apps connect with an RDS IAM token.
_Avoid_: Config role, CDC as owner, owner secret, org-set password

**CDC role**:
Postgres LOGIN role Apply creates for this Config (`{prefix}_{database}_cdc`). Has `rds_iam`, `rds_replication`, and SELECT on this Config's Tables. Debezium uses it. Isolated to this Config's Database.
_Avoid_: Config role, replication as owner, CDC secret, org-set password

**Connect source role**:
IAM role Apply creates for this Config's Debezium connector (`relay-connect-{prefix}-cdc`). Iceberg uses the Cluster Connect role. Only the source role may `rds-db:connect` as this CDC role.
