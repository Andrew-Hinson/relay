# Relay

PaaS CLI. A service team commits a Config YAML and runs one command. Relay creates the RDS Instance (or attaches to one), one Database, the Tables, and a streaming path: Postgres WAL → Debezium → MSK topic → Iceberg in the Cluster Warehouse.

The Cluster (MSK, Connect, S3 + Glue) already exists. The Config never creates it. The app writes rows. YAML owns DDL. Re-Apply fails if live columns drift. No destroy. No BI.

See [CONTEXT.md](CONTEXT.md) for terms.

## Config

[example/example.yaml](example/example.yaml):

```yaml
apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true          # or create: false and name: existing-instance
database: {}            # name defaults to Config name + db
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
      - name: amount
        type: numeric
        nullable: false
```

Optional `prefix:` (default: Config name). Optional `kafka:` (`partitions`, `replicas`, `min.insync.replicas`; defaults 3/3/2). Unknown keys are rejected.

Org/secops creates a Secrets Manager secret named after the Instance (`user`, `password`, optional `host`) before Apply. YAML has no secret fields. Attach (`instance.create: false`) uses that secret to bootstrap SQL. Create (`instance.create: true`) uses the username; RDS manages the master password; Apply fetches that RDS secret to bootstrap only. Apply then creates a Config role and Config role secret. Debezium and the printed connection use that secret. Terraform state stores secret ARNs, not passwords.

The Debezium plugin ZIP must include the [MSK config-providers JAR](https://github.com/aws-samples/msk-config-providers/releases). `--msk-bootstrap-servers` must be the IAM listeners (port 9098). Org Connect role is Iceberg only: Warehouse S3/Glue, plugin S3, logs, MSK Connect internals (`__amazon_msk_connect_*`). No Secrets Manager. Apply attaches `kafka-cluster` on `{prefix}*` topics and groups to that role for the sink.

Apply creates IAM role `relay-connect-{prefix}-cdc` for Debezium, under `--connect-source-boundary-arn`, and attaches `--connect-worker-policy-arn` (logs, plugin S3, Connect internals). That role gets `kafka-cluster` on `{prefix}*` and `secretsmanager:GetSecretValue`/`DescribeSecret` on this Config role secret (`relay/{cluster}/{name}/cdc`). Connect VPC needs a path to Secrets Manager. The Apply caller uses the default AWS credential chain for MSK IAM (topics), `secretsmanager:GetSecretValue`/`DescribeSecret`/`CreateSecret`, `iam:CreateRole`, `iam:PutRolePolicy`, `iam:AttachRolePolicy` on the worker policy, `iam:PassRole` on the source role to `kafkaconnect.amazonaws.com`, and `iam:PutRolePolicy` on the org Connect role (kafka prefix only).

Org also creates one Terraform state bucket per Cluster (versioning, encryption, Apply-caller IAM only). Not the Warehouse bucket. Apply stores state at `relay/<cluster>/<name>/terraform.tfstate`.

## Apply

Reads the Config, applies Table DDL, and runs Terraform. State lives in the Cluster state bucket. `.relay/` (gitignored) is tfvars, SQL, and plugin cache. You do not run Terraform.

```bash
# Parse example.yaml, create/attach Instance, apply DDL, attach CDC + Iceberg on the Cluster.
# Equivalent env vars: RELAY_REGION, RELAY_MSK_BOOTSTRAP_SERVERS, …
go run -C cmd/relay . apply -f example/example.yaml \
  --region us-east-1 \
  --msk-bootstrap-servers "$BOOTSTRAP" \
  --msk-cluster-arn "$MSK_ARN" \
  --warehouse-bucket "$BUCKET" \
  --state-bucket "$STATE_BUCKET" \
  --glue-database "$GLUE_DB" \
  --debezium-plugin-arn "$DEBEZIUM_PLUGIN" \
  --iceberg-plugin-arn "$ICEBERG_PLUGIN" \
  --connect-role-arn "$CONNECT_ROLE" \
  --connect-source-boundary-arn "$CONNECT_SOURCE_BOUNDARY" \
  --connect-worker-policy-arn "$CONNECT_WORKER_POLICY" \
  --connect-subnet-ids "$CONNECT_SUBNETS" \
  --connect-sg-ids "$CONNECT_SGS"
```

When `instance.create` is true, also pass `--subnet-ids`, `--rds-sg-ids`. Optional: `--rds-instance-class` (default `db.t3.medium`), `--rds-engine-version` (default `16`). One-shot `--migrate-state` copies a leftover `.relay/<name>/terraform.tfstate` into the Cluster bucket when that key is empty.

Prints `endpoint`, `database`, `user`, `secret`. Never the password.

## Dev

```bash
# Unit tests for parse, plan, and Apply file layout.
go test -C cmd/relay ./...

# Format check and validate the Terraform modules Apply runs.
terraform fmt -check $(git ls-files '*.tf')
terraform -chdir=tf init -backend=false
terraform -chdir=tf validate
```
