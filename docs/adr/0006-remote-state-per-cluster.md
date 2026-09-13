# Remote state per Cluster

Apply stores Terraform state in an org-created S3 bucket per Cluster, key `relay/<cluster>/<name>/terraform.tfstate`, with S3 lockfile. Local `.relay/<name>/terraform.tfstate` was rejected: two laptops or a lost dir split-brain a shared Cluster. The Warehouse bucket was rejected so Connect cannot read or write state. `--state-bucket` is required; `--migrate-state` is a one-shot copy of leftover local state when the key is empty.
