# Config role owns DDL and CDC

Superseded by 0009. Apply created a per-Config Postgres role (`{prefix}_{database}_cdc`) with LOGIN and `rds_replication`. Table DDL and Debezium ran as that role. That was rejected: the CDC login owned tables, so stolen CDC credentials could DDL/DML, and the printed connection had replication.
