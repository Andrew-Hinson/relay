package main

import (
	"strings"
	"testing"
)

func TestCreateSecretArgs_passesNameNotInQuery(t *testing.T) {
	got := strings.Join(createSecretArgs("relay/prod/acme/cdc", `{"username":"acme_acmedb_cdc","password":"x"}`, "eu-west-1"), " ")
	if !strings.Contains(got, "secretsmanager create-secret --name relay/prod/acme/cdc") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--query ARN") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestRandomPassword_lengthAndCharset(t *testing.T) {
	got, err := randomPassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != passwordLen {
		t.Fatalf("got len %d", len(got))
	}
	if strings.ContainsAny(got, `'"\`) {
		t.Fatalf("got metachar %q", got)
	}
}

func TestSecretNotFound_detectsAWS(t *testing.T) {
	if !secretNotFound("An error occurred (ResourceNotFoundException) when calling the DescribeSecret operation") {
		t.Fatal("expected not found")
	}
	if secretNotFound("AccessDenied") {
		t.Fatal("denied is not not-found")
	}
}

func TestSecretARNArgs_passesRegion(t *testing.T) {
	got := strings.Join(secretARNArgs("shared-rds", "eu-west-1"), " ")
	if !strings.Contains(got, "secretsmanager describe-secret --secret-id shared-rds") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--query ARN") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestSecretValueArgs_passesRegion(t *testing.T) {
	got := strings.Join(secretValueArgs("shared-rds", "eu-west-1"), " ")
	if !strings.Contains(got, "--secret-id shared-rds") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestParseInstanceSecret_orgFormat(t *testing.T) {
	creds, err := parseInstanceSecret([]byte(`{"user":"relay","password":"s3cret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if creds.User != "relay" || creds.Password != "s3cret" {
		t.Fatalf("got %+v", creds)
	}
}

func TestParseInstanceSecret_rdsManagedFormat(t *testing.T) {
	creds, err := parseInstanceSecret([]byte(`{"username":"relay","password":"s3cret","engine":"postgres"}`))
	if err != nil {
		t.Fatal(err)
	}
	if creds.User != "relay" || creds.Password != "s3cret" {
		t.Fatalf("got %+v", creds)
	}
}

func TestParseInstanceSecret_requiresLogin(t *testing.T) {
	if _, err := parseInstanceSecret([]byte(`{"host":"db.example"}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestSecretIDFromARN_stripsPrefix(t *testing.T) {
	arn := "arn:aws:secretsmanager:us-east-1:000000000000:secret:rds!db-acme-AbCdEf"
	if got := secretIDFromARN(arn); got != "rds!db-acme-AbCdEf" {
		t.Fatalf("got %q", got)
	}
	if got := secretIDFromARN("shared-rds"); got != "shared-rds" {
		t.Fatalf("got %q", got)
	}
}

func TestBindMasterSecret_doesNotPointDebeziumAtMaster(t *testing.T) {
	login := instanceLogin{User: "org", Password: "org-pw"}
	plan := applyPlan{Connection: plannedConnection{User: "acme_acmedb_cdc", Secret: "relay/prod/acme/cdc", SecretUserKey: "username"}}
	bindMasterSecret(&login, &plan, instanceCreds{User: "relay", Password: "master-pw"}, "db.example")
	if login.User != "relay" || login.Password != "master-pw" {
		t.Fatalf("login %+v", login)
	}
	if login.Host != "db.example" {
		t.Fatalf("got host %q", login.Host)
	}
	if plan.Instance.Username != "relay" {
		t.Fatalf("got instance user %q", plan.Instance.Username)
	}
	if plan.Connection.User != "acme_acmedb_cdc" {
		t.Fatalf("got connection user %q", plan.Connection.User)
	}
	if plan.Connection.Secret != "relay/prod/acme/cdc" {
		t.Fatalf("got secret %q", plan.Connection.Secret)
	}
}

func TestBindConfigSecret_setsRuntimeARN(t *testing.T) {
	plan := applyPlan{Connection: plannedConnection{User: "acme_acmedb_cdc", Secret: "relay/prod/acme/cdc"}}
	arn := "arn:aws:secretsmanager:us-east-1:000000000000:secret:relay/prod/acme/cdc-AbCdEf"
	bindConfigSecret(&plan, instanceCreds{User: "acme_acmedb_cdc"}, arn)
	if plan.Connection.SecretARN != arn {
		t.Fatalf("got arn %q", plan.Connection.SecretARN)
	}
	if plan.Connection.SecretUserKey != "username" {
		t.Fatalf("got key %q", plan.Connection.SecretUserKey)
	}
}

func TestDescribeDBInstanceArgs_passesRegion(t *testing.T) {
	got := strings.Join(describeDBInstanceArgs("shared-rds", "eu-west-1"), " ")
	if !strings.Contains(got, "--db-instance-identifier shared-rds") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}
