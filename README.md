# Relay

PaaS CLI. Commit a [Config](example/example.yaml), run Apply. Relay attaches it to an existing Cluster: Instance, Database, Tables, and a path to Iceberg.

```bash
go run -C cmd/relay . apply -f example/example.yaml
go test -C cmd/relay ./...
```

See [CONTEXT.md](CONTEXT.md) for terms.

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


