# IAM for every login, no secrets

Passwords and Secrets Manager were rejected: Debezium fetched a CDC secret, Apply printed an owner secret name, and create read the RDS-managed master secret once to `GRANT rds_iam`. Apply, owner, and CDC now authenticate with RDS IAM tokens. Create uses `MasterUserAuthenticationType=iam-db-auth` so the master user is IAM from birth. The Connect source role is Debezium's identity (`rds-db:connect` on this CDC user), not a secret reader.

Supersedes the secret-passing parts of 0003, 0007, and 0009.
