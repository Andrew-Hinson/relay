# Relay

PaaS CLI. Commit a [Config](example/example.yaml), run Apply. Relay attaches it to an existing Cluster: Instance, Database, Tables, and a path to Iceberg.

Install
```bash
go install -C cmd/relay .
go test -C cmd/relay ./...
```

Run
```
mkdir -p clusters && cp example/cluster.yaml clusters/prod.yaml   # once per Cluster; fill in real ARNs and IDs
relay plan example/example.yaml
relay apply example/example.yaml
```

A Config's `cluster: prod` loads the Cluster profile `clusters/prod.yaml` (or `~/.config/relay/clusters/prod.yaml`). Flags and `RELAY_*` env vars override any field. Relay refuses to run if an ARN is in another account or region, or if your AWS credentials are for a different account than the profile.

Apply shows the Terraform plan and the SQL it will run, then asks for `yes` before changing anything. It applies exactly the reviewed plan. In CI, pass `--yes`; without a terminal Apply refuses. Creating an Instance asks twice: once for the Instance, then for the rest.

Kafka topics and Glue Iceberg tables have `prevent_destroy`. Removing a Table from a Config fails at plan until you drop it deliberately (`terraform state rm` of its topic and Glue table).

Apply stamps the Database and its owner/CDC roles with `COMMENT ... IS 'relay:{cluster}/{config}'` and refuses objects stamped by another Config. Configs applied before stamping existed: run `relay apply --adopt <config.yaml>` once. `relay plan --adopt` lists what will be claimed. Adopt only claims objects when the Database is already owned by this Config's owner role.

Postgres connections verify the Instance certificate (`sslmode=verify-full`) against the embedded [RDS CA bundle](cmd/relay/rds-global-bundle.pem).

The Config's `cluster` must match the cluster name in `RELAY_MSK_CLUSTER_ARN`.

See [CONTEXT.md](CONTEXT.md) for terms. See [aws-account.md](aws-account.md) for Cluster IAM and account setup.

```mermaid
flowchart TB
  Config[Config] -->|Apply| T[Tables]
  App[App] -->|owner role| T

  subgraph inst [Instance]
    T
  end

  subgraph cl [Cluster]
    direction TB
    Deb[Debezium]
    MSK
    Sink[Iceberg sink]
    Ice[Iceberg table]
    Deb -->|topic| MSK --> Sink --> Ice
  end

  T -->|CDC role, WAL| Deb
  Ice --> BI[BI]
```


