# Relay

A Config is the YAML a service team commits. Apply attaches that Config to an existing Cluster: an Instance, one Database, Tables, and a streaming path to Iceberg.

## Language

**Config**:
The YAML a service team commits. Input to Apply. Identifiers are `[a-z][a-z0-9_]*`. An Instance name used on create is `[a-z][a-z0-9]*`.
_Avoid_: Project, manifest, Relay file

**Apply**:
Reconciliation of a Config. v1 has no destroy.

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

**Instance Secret**:
The Secrets Manager store named after the Instance. Org creates it. Apply retrieves it to bootstrap (CREATE DATABASE, CREATE ROLE, GRANT). On create, RDS manages the master password; Apply fetches that for bootstrap only. Not used by Debezium. Not in Config. Not in Terraform state.

**Config role**:
Postgres LOGIN role Apply creates for this Config (`{prefix}_{database}_cdc`). Owns the Database and Tables. Has `rds_replication`. Apply runs Table DDL as this role. Debezium uses it. Isolated to this Config's Database (`REVOKE CONNECT FROM PUBLIC`).

**Config role secret**:
Secrets Manager secret Apply creates at `relay/{cluster}/{name}/cdc` (`username`, `password`). Debezium and the printed connection use it. The Connect source role may `GetSecretValue` on this secret only. Not in Config. Not in Terraform state.

**Connect source role**:
IAM role Apply creates for this Config's Debezium connector (`relay-connect-{prefix}-cdc`). Iceberg uses the Cluster Connect role. Only the source role may `GetSecretValue` on this Config role secret.
