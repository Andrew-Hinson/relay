# Relay

A Config is the YAML a service team commits. Apply attaches that Config to an existing Cluster: an Instance, one Database, Tables, and a streaming path to Iceberg.

## Language

**Config**:
The YAML a service team commits. Input to Apply.
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
The derived lake relation for a Table. Name is Glue-safe `[a-z0-9_]` (`example-service` + `public.orders` → `example_service_public_orders`). BI reads it. Not a Relay object.

**Instance**:
The RDS server a Config's Database lives on. A Config creates it or names an existing one.
_Avoid_: cluster, RDS as the generic term

**Database**:
A named Postgres database on an Instance. This Config always creates it. One per Config. Share happens at Instance only. Names are lowercased with hyphens removed. Default is the Config name plus a `db` suffix (`example-service` → `exampleservicedb`). An override is sanitized the same way (`shop-db` → `shopdb`).
_Avoid_: schema, database.create false

**Schema**:
A Postgres namespace inside a Database. Default `public`.

**Table**:
A named relation this Config owns. Columns are DDL. A primary key is required. The app writes rows only.

**Prefix**:
A Config override for resource names, defaulting to the Config name. Topics use `{prefix}.{schema}.{table}`.

**Instance Secret**:
The Secrets Manager store named after the Instance. Org creates it. Apply retrieves it on attach. On create, RDS manages the master secret; Apply and Debezium fetch that. Not in Config. Not in Terraform state.
