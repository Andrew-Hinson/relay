# Apply SQL uses IAM auth tokens

Apply logs in to Postgres as the instance master user with `rds generate-db-auth-token`, not a stored password. Config YAML has no secret fields. Create uses `MasterUserAuthenticationType=iam-db-auth` so the master user is IAM from birth; Apply never calls Secrets Manager. Password login for Apply was rejected so the CLI does not hold a live master password for DDL.

Secret names and a create-time Secrets Manager bootstrap are superseded by 0010.
