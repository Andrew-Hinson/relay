# Owner role owns DDL, CDC role replicates

A single Config role that owned tables and had `rds_replication` was rejected: stolen CDC credentials could DDL/DML, and the printed app login had replication. Apply creates an owner role (`{prefix}_{database}`) for table ownership and a CDC role (`{prefix}_{database}_cdc`) with `LOGIN`, `rds_iam`, `rds_replication`, `CONNECT`, `USAGE`, and `SELECT` only. Apply does not set passwords. Debezium publication autocreate was rejected so CDC does not need `CREATE`. Apply creates the publication from YAML. Printed connection is the owner role with IAM auth; Connect IAM is `rds-db:connect` on the CDC user only.

Supersedes 0008. Owner and CDC secret names are superseded by 0010.
