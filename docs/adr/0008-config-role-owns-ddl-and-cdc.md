# Config role owns DDL and CDC

Apply creates a per-Config Postgres role (`{prefix}_{database}_cdc`) with LOGIN and `rds_replication`. Table DDL and Debezium run as that role. The Instance Secret / RDS master is bootstrap only: CREATE DATABASE, CREATE ROLE, GRANT. Debezium using the instance login was rejected: on a shared Instance that login can read every database and publish tables it does not own.

The Config role secret (`relay/{cluster}/{name}/cdc`) is Apply-created. Instance Secret stays org-owned (0003). Password is not in Config, tfvars, apply.sql, or stdout.
