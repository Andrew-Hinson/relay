# AWS account

Relay does not create a Cluster. `relay apply` attaches a Config to objects that already exist. Names below match repo defaults. Rename freely if the `RELAY_*` ARNs you pass still point at them.

## Roles at a glance

| Role | Kind | Who creates | Used by |
| --- | --- | --- | --- |
| Apply identity | IAM user/role | you | `relay plan` / `relay apply` |
| `relay-connect` | IAM role | you | Iceberg sink (Cluster Connect role) |
| `relay-connect-{prefix}-cdc` | IAM role | Apply | Debezium (Connect source role) |
| `{prefix}_{database}` | Postgres LOGIN | Apply | app writes (`rds_iam`) |
| `{prefix}_{database}_cdc` | Postgres LOGIN | Apply | Debezium (`rds_iam`, `rds_replication`) |
| Instance master (`relay` on create) | Postgres LOGIN | RDS / you | Apply SQL |

Three customer-managed policies must exist before Apply:

| Policy | Attached to |
| --- | --- |
| `relay-connect-worker` | every Connect source role; attach to `relay-connect` too |
| `relay-connect-source-boundary` | permissions boundary on every Connect source role |
| `relay-connect-sink-boundary` | permissions boundary on `relay-connect` |

## Bootstrap

Do these once per account/region Cluster. Then set `RELAY_*` and run Apply.

1. VPC with 2+ private subnets. MSK, Connect, and RDS share it.
2. MSK provisioned cluster, 3 brokers if you keep default `replicas: 3`. IAM auth on. TLS on. Save the IAM bootstrap broker string and cluster ARN.
3. S3 warehouse bucket. Glue database (Iceberg catalog).
4. S3 state bucket. Apply writes `relay/{cluster}/{config}/terraform.tfstate` with a lockfile.
5. Custom plugins: upload Debezium Postgres + Iceberg sink JARs, create two MSK Connect custom plugins, save ARNs.
6. Security groups:
   - Connect SG: egress 9098 to MSK, egress 5432 to RDS.
   - MSK SG: ingress 9098 from Connect SG and from the Apply host.
   - RDS SG: ingress 5432 from Connect SG and from the Apply host.
7. Create `relay-connect-worker`, `relay-connect-source-boundary`, and `relay-connect-sink-boundary` (JSON below).
8. Create `relay-connect` with `relay-connect-sink-boundary` as its permissions boundary. Trust `kafkaconnect.amazonaws.com`. Attach worker + warehouse policies. Apply later puts `{prefix}-topics` inline on this role.
9. Create the Apply identity (JSON below). It must reach RDS:5432 from where you run the CLI.
10. Export Cluster env. `relay apply` fails closed if any required value is missing.

```bash
export RELAY_REGION=us-east-1
export RELAY_MSK_BOOTSTRAP_SERVERS=b-1.xxx:9098,b-2.xxx:9098,b-3.xxx:9098
export RELAY_MSK_CLUSTER_ARN=arn:aws:kafka:us-east-1:ACCOUNT:cluster/prod/UUID
export RELAY_WAREHOUSE_BUCKET=relay-warehouse
export RELAY_STATE_BUCKET=relay-tfstate
export RELAY_GLUE_DATABASE=relay
export RELAY_DEBEZIUM_PLUGIN_ARN=arn:aws:kafkaconnect:us-east-1:ACCOUNT:custom-plugin/debezium/UUID
export RELAY_ICEBERG_PLUGIN_ARN=arn:aws:kafkaconnect:us-east-1:ACCOUNT:custom-plugin/iceberg/UUID
export RELAY_CONNECT_ROLE_ARN=arn:aws:iam::ACCOUNT:role/relay-connect
export RELAY_CONNECT_SOURCE_BOUNDARY_ARN=arn:aws:iam::ACCOUNT:policy/relay-connect-source-boundary
export RELAY_CONNECT_WORKER_POLICY_ARN=arn:aws:iam::ACCOUNT:policy/relay-connect-worker
export RELAY_CONNECT_SUBNET_IDS=subnet-aaa,subnet-bbb
export RELAY_CONNECT_SG_IDS=sg-connect
# only when Config instance.create is true
export RELAY_SUBNET_IDS=subnet-aaa,subnet-bbb
export RELAY_RDS_SG_IDS=sg-rds
```

Attach Instance extra: IAM database auth on, `rds.logical_replication=1`, `rds.iam_auth_for_replication=1`. Create sets those.

## Cluster Connect role (`relay-connect`)

Iceberg sink `service_execution_role_arn`. Apply does not create it. Apply does `iam:PutRolePolicy` on it, name `{prefix}-topics` (dots become dashes). Apply may only do so while `relay-connect-sink-boundary` is its boundary, so the Apply identity cannot use this role to escalate.

Trust (confused-deputy conditions required by MSK Connect):

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "kafkaconnect.amazonaws.com" },
    "Action": "sts:AssumeRole",
    "Condition": {
      "StringEquals": { "aws:SourceAccount": "ACCOUNT" },
      "ArnLike": {
        "aws:SourceArn": "arn:aws:kafkaconnect:REGION:ACCOUNT:connector/*"
      }
    }
  }]
}
```

Attach `relay-connect-worker`. Add warehouse (not on the worker policy, or source roles inherit S3/Glue):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "GlueIceberg",
      "Effect": "Allow",
      "Action": [
        "glue:GetDatabase",
        "glue:GetDatabases",
        "glue:GetTable",
        "glue:GetTables",
        "glue:UpdateTable",
        "glue:GetPartition",
        "glue:GetPartitions",
        "glue:BatchCreatePartition",
        "glue:BatchGetPartition"
      ],
      "Resource": [
        "arn:aws:glue:REGION:ACCOUNT:catalog",
        "arn:aws:glue:REGION:ACCOUNT:database/relay",
        "arn:aws:glue:REGION:ACCOUNT:table/relay/*"
      ]
    },
    {
      "Sid": "Warehouse",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::relay-warehouse"
    },
    {
      "Sid": "WarehouseObjects",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::relay-warehouse/*"
    }
  ]
}
```

If Lake Formation governs that Glue database, also `lakeformation:GetDataAccess` and LF insert/alter on the tables. IAM-only is enough when LF is not enforcing.

Apply then adds, per Config prefix, the same Kafka data-plane as the source role: cluster Connect/Describe/WriteDataIdempotently; topic Create/Describe/Read/Write on `{prefix}.*`; group Alter/Describe on `{prefix}-*` and `connect-{prefix}-*`. That covers `{prefix}.{schema}.{table}` and `{prefix}.control.iceberg`. Wildcards are anchored on `.` / `-` so prefix `ex` never matches `example`.

## Worker policy (`relay-connect-worker`)

MSK Connect internals. Not in this repo's inline policies. AWS requires these on every service execution role talking IAM-auth MSK. [Service execution role](https://docs.aws.amazon.com/msk/latest/developerguide/msk-connect-service-execution-role.html).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "Cluster",
      "Effect": "Allow",
      "Action": [
        "kafka-cluster:Connect",
        "kafka-cluster:DescribeCluster",
        "kafka-cluster:WriteDataIdempotently"
      ],
      "Resource": "arn:aws:kafka:REGION:ACCOUNT:cluster/prod/UUID"
    },
    {
      "Sid": "ConnectInternalTopics",
      "Effect": "Allow",
      "Action": [
        "kafka-cluster:CreateTopic",
        "kafka-cluster:DescribeTopic",
        "kafka-cluster:ReadData",
        "kafka-cluster:WriteData"
      ],
      "Resource": "arn:aws:kafka:REGION:ACCOUNT:topic/prod/UUID/__amazon_msk_connect_*"
    },
    {
      "Sid": "ConnectInternalGroups",
      "Effect": "Allow",
      "Action": ["kafka-cluster:AlterGroup", "kafka-cluster:DescribeGroup"],
      "Resource": [
        "arn:aws:kafka:REGION:ACCOUNT:group/prod/UUID/__amazon_msk_connect_*",
        "arn:aws:kafka:REGION:ACCOUNT:group/prod/UUID/connect-*"
      ]
    }
  ]
}
```

Terraform does not set connector log delivery. Add `logs:CreateLogGroup|CreateLogStream|PutLogEvents|DescribeLogGroups|DescribeLogStreams` on `/aws/msk-connect/*` if you enable it later.

## Sink boundary (`relay-connect-sink-boundary`)

Ceiling on `relay-connect`. Must allow everything that role actually uses (worker + every prefix's Kafka + warehouse). Whatever Apply writes inline, the role can never exceed this.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "MskThisCluster",
      "Effect": "Allow",
      "Action": "kafka-cluster:*",
      "Resource": [
        "arn:aws:kafka:REGION:ACCOUNT:cluster/prod/UUID",
        "arn:aws:kafka:REGION:ACCOUNT:topic/prod/UUID/*",
        "arn:aws:kafka:REGION:ACCOUNT:group/prod/UUID/*"
      ]
    },
    {
      "Sid": "GlueIceberg",
      "Effect": "Allow",
      "Action": [
        "glue:GetDatabase",
        "glue:GetDatabases",
        "glue:GetTable",
        "glue:GetTables",
        "glue:UpdateTable",
        "glue:GetPartition",
        "glue:GetPartitions",
        "glue:BatchCreatePartition",
        "glue:BatchGetPartition"
      ],
      "Resource": [
        "arn:aws:glue:REGION:ACCOUNT:catalog",
        "arn:aws:glue:REGION:ACCOUNT:database/relay",
        "arn:aws:glue:REGION:ACCOUNT:table/relay/*"
      ]
    },
    {
      "Sid": "Warehouse",
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::relay-warehouse", "arn:aws:s3:::relay-warehouse/*"]
    }
  ]
}
```

Add `lakeformation:GetDataAccess` here too if Lake Formation governs the Glue database.

## Source boundary (`relay-connect-source-boundary`)

Ceiling on `relay-connect-{prefix}-cdc`. Must allow everything that role actually uses (worker + prefix Kafka + `rds-db:connect` as `*_cdc`). Must omit warehouse S3 and Glue.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "MskThisCluster",
      "Effect": "Allow",
      "Action": "kafka-cluster:*",
      "Resource": [
        "arn:aws:kafka:REGION:ACCOUNT:cluster/prod/UUID",
        "arn:aws:kafka:REGION:ACCOUNT:topic/prod/UUID/*",
        "arn:aws:kafka:REGION:ACCOUNT:group/prod/UUID/*"
      ]
    },
    {
      "Sid": "CdcIamAuth",
      "Effect": "Allow",
      "Action": "rds-db:connect",
      "Resource": "arn:aws:rds-db:REGION:ACCOUNT:dbuser:*/*_cdc"
    }
  ]
}
```

## Connect source role (Apply creates)

Name `relay-connect-{prefix}-cdc` (`.` in prefix becomes `-`). Example Config `example` → `relay-connect-example-cdc`.

- Trust: `kafkaconnect.amazonaws.com`, `aws:SourceAccount=ACCOUNT`, `aws:SourceArn=arn:aws:kafkaconnect:REGION:ACCOUNT:connector/{prefix}-{database}-cdc/*`
- `permissions_boundary` = `relay-connect-source-boundary`
- Attach `relay-connect-worker`
- Inline `{prefix}-topics`: Kafka on `{prefix}.*` topics and `{prefix}-*` / `connect-{prefix}-*` groups
- Inline `{prefix}-rds`: `rds-db:connect` on `arn:aws:rds-db:REGION:ACCOUNT:dbuser:{instance-resource-id}/{prefix}_{database}_cdc`

Only this role may connect as the CDC Postgres role.

Debezium connects with `database.sslmode=require`: encrypted, but the RDS certificate is not verified. `relay` itself verifies (`verify-full` against the embedded RDS CA bundle). Moving Debezium to `verify-full` needs the RDS CA bundle in the Connect worker truststore.

## Apply identity

Whoever runs the CLI. AWS API plus a network path to RDS:5432.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "PlanRds",
      "Effect": "Allow",
      "Action": ["rds:DescribeDBInstances"],
      "Resource": "*"
    },
    {
      "Sid": "MasterIamAuth",
      "Effect": "Allow",
      "Action": "rds-db:connect",
      "Resource": "arn:aws:rds-db:REGION:ACCOUNT:dbuser:*/*"
    },
    {
      "Sid": "State",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::relay-tfstate"
    },
    {
      "Sid": "StateObjects",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::relay-tfstate/relay/*"
    },
    {
      "Sid": "CreateInstance",
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBInstance",
        "rds:CreateDBSubnetGroup",
        "rds:CreateDBParameterGroup",
        "rds:DescribeDBSubnetGroups",
        "rds:DescribeDBParameterGroups",
        "rds:DescribeDBParameters",
        "rds:AddTagsToResource",
        "rds:ListTagsForResource",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeVpcs"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SourceRole",
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:GetRole",
        "iam:GetRolePolicy",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:AttachRolePolicy",
        "iam:ListAttachedRolePolicies",
        "iam:ListRolePolicies",
        "iam:TagRole"
      ],
      "Resource": "arn:aws:iam::ACCOUNT:role/relay-connect-*-cdc",
      "Condition": {
        "StringEquals": {
          "iam:PermissionsBoundary": "arn:aws:iam::ACCOUNT:policy/relay-connect-source-boundary"
        }
      }
    },
    {
      "Sid": "AclOnConnectRole",
      "Effect": "Allow",
      "Action": ["iam:GetRole", "iam:GetRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy"],
      "Resource": "arn:aws:iam::ACCOUNT:role/relay-connect",
      "Condition": {
        "StringEquals": {
          "iam:PermissionsBoundary": "arn:aws:iam::ACCOUNT:policy/relay-connect-sink-boundary"
        }
      }
    },
    {
      "Sid": "PassConnectRoles",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": [
        "arn:aws:iam::ACCOUNT:role/relay-connect",
        "arn:aws:iam::ACCOUNT:role/relay-connect-*-cdc"
      ],
      "Condition": {
        "StringEquals": { "iam:PassedToService": "kafkaconnect.amazonaws.com" }
      }
    },
    {
      "Sid": "ConnectApi",
      "Effect": "Allow",
      "Action": [
        "kafkaconnect:CreateConnector",
        "kafkaconnect:UpdateConnector",
        "kafkaconnect:DescribeConnector",
        "kafkaconnect:ListConnectors",
        "kafkaconnect:CreateWorkerConfiguration",
        "kafkaconnect:DescribeWorkerConfiguration",
        "kafkaconnect:ListWorkerConfigurations",
        "ec2:CreateNetworkInterface",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DeleteNetworkInterface"
      ],
      "Resource": "*"
    },
    {
      "Sid": "GlueTables",
      "Effect": "Allow",
      "Action": [
        "glue:GetDatabase",
        "glue:GetTable",
        "glue:CreateTable",
        "glue:UpdateTable",
        "glue:GetTables"
      ],
      "Resource": [
        "arn:aws:glue:REGION:ACCOUNT:catalog",
        "arn:aws:glue:REGION:ACCOUNT:database/relay",
        "arn:aws:glue:REGION:ACCOUNT:table/relay/*"
      ]
    },
    {
      "Sid": "KafkaTopics",
      "Effect": "Allow",
      "Action": [
        "kafka-cluster:Connect",
        "kafka-cluster:DescribeCluster",
        "kafka-cluster:CreateTopic",
        "kafka-cluster:DescribeTopic",
        "kafka-cluster:AlterTopic"
      ],
      "Resource": [
        "arn:aws:kafka:REGION:ACCOUNT:cluster/prod/UUID",
        "arn:aws:kafka:REGION:ACCOUNT:topic/prod/UUID/*"
      ]
    }
  ]
}
```

`MasterIamAuth` is wide (`dbuser:*/*`) so Apply can log in as whatever `MasterUsername` the Instance has. Narrow to `dbuser:*/relay` if every Instance uses that master.

Create-Instance block is unused when Configs only attach.

Encrypted RDS uses the AWS-managed RDS key unless you set a CMK. A CMK needs `kms:CreateGrant` / `kms:DescribeKey` on Apply.

## App identity

Not created by Relay. Each writer needs:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "rds-db:connect",
    "Resource": "arn:aws:rds-db:REGION:ACCOUNT:dbuser:INSTANCE_RESOURCE_ID/PREFIX_DATABASE"
  }]
}
```

Example Config `example` / database `exampledb` → user `example_exampledb`. Apply prints `user` + `auth: iam`. Token: `aws rds generate-db-auth-token`.

Do not grant apps `rds-db:connect` as `*_cdc`.

## Postgres roles Apply creates

| Role | Grants | IAM |
| --- | --- | --- |
| `{prefix}_{database}` | owns Database + Tables; `CONNECT, CREATE` on Database; `USAGE, CREATE` on Schema | `GRANT rds_iam` |
| `{prefix}_{database}_cdc` | `CONNECT` on Database; `USAGE` on Schema; `SELECT` on Tables; `rds_replication` | `GRANT rds_iam` |

`REVOKE CONNECT ON DATABASE ... FROM PUBLIC`. Only owner, CDC, and master reach that Database.

## Apply-created AWS objects (do not pre-create)

- IAM role `relay-connect-{prefix}-cdc`
- MSK Connect connector `{prefix}-{database}-cdc` (Debezium)
- MSK Connect worker configuration `{prefix}-{database}-cdc-worker`
- MSK Connect connector `{prefix}-{database}-iceberg`
- Kafka topics `{prefix}.{schema}.{table}`, `{prefix}.control.iceberg`
- Glue Iceberg tables `{prefix}_{schema}_{table}` in `RELAY_GLUE_DATABASE`
- Optional RDS Instance `{instance.name}` (master `relay`, IAM auth from birth)
