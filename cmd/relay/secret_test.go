package main

import (
	"strings"
	"testing"
)

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

func TestBindMasterSecret_runtimePointsAtRDS(t *testing.T) {
	login := instanceLogin{User: "org", Password: "org-pw"}
	plan := applyPlan{Connection: plannedConnection{Secret: "acme", SecretUserKey: "user"}}
	arn := "arn:aws:secretsmanager:us-east-1:000000000000:secret:rds!db-acme-AbCdEf"
	bindMasterSecret(&login, &plan, instanceCreds{User: "relay", Password: "master-pw"}, "db.example", arn)
	if login.User != "relay" || login.Password != "master-pw" {
		t.Fatalf("login %+v", login)
	}
	if plan.Connection.Secret != "rds!db-acme-AbCdEf" {
		t.Fatalf("got secret %q", plan.Connection.Secret)
	}
	if plan.Connection.SecretUserKey != "username" {
		t.Fatalf("got key %q", plan.Connection.SecretUserKey)
	}
	if plan.Connection.SecretARN != arn {
		t.Fatalf("got arn %q", plan.Connection.SecretARN)
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
