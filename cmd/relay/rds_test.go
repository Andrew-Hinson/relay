package main

import (
	"errors"
	"strings"
	"testing"
)

func TestInstanceNotFound(t *testing.T) {
	if !instanceNotFound(errors.New("instance acme: An error occurred (DBInstanceNotFound) when calling the DescribeDBInstances operation")) {
		t.Fatal("expected not-found")
	}
	if instanceNotFound(errors.New("instance acme: AccessDenied")) {
		t.Fatal("access denied is not missing")
	}
}

func TestParseDBInstance_requiresIAM(t *testing.T) {
	info, err := parseDBInstance([]byte(`{"host":"db.example","user":"relay","iam":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if info.Host != "db.example" || info.User != "relay" {
		t.Fatalf("got %+v", info)
	}
	if _, err := parseDBInstance([]byte(`{"host":"db.example","user":"relay","iam":false}`)); err == nil {
		t.Fatal("expected IAM off error")
	}
}

func TestDescribeDBInstanceArgs_queriesIAM(t *testing.T) {
	got := strings.Join(describeDBInstanceArgs("shared-rds", "eu-west-1"), " ")
	if !strings.Contains(got, "--db-instance-identifier shared-rds") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "MasterUsername") || !strings.Contains(got, "IAMDatabaseAuthenticationEnabled") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateDBAuthTokenArgs_passesHostUserPort(t *testing.T) {
	got := strings.Join(generateDBAuthTokenArgs("db.example", "relay", "eu-west-1"), " ")
	if !strings.Contains(got, "rds generate-db-auth-token --hostname db.example --port 5432 --username relay") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestBindInstanceLogin_doesNotPointDebeziumAtMaster(t *testing.T) {
	login := instanceLogin{User: "org", Password: "token"}
	plan := applyPlan{
		Connection: plannedConnection{User: "acme_acmedb"},
		CDC:        plannedCDC{User: "acme_acmedb_cdc"},
	}
	bindInstanceLogin(&login, &plan, "db.example", "relay")
	if login.User != "relay" {
		t.Fatalf("login %+v", login)
	}
	if login.Host != "db.example" {
		t.Fatalf("got host %q", login.Host)
	}
	if plan.Instance.Username != "relay" {
		t.Fatalf("got instance user %q", plan.Instance.Username)
	}
	if plan.Connection.User != "acme_acmedb" {
		t.Fatalf("got connection user %q", plan.Connection.User)
	}
	if plan.CDC.User != "acme_acmedb_cdc" {
		t.Fatalf("got CDC user %q", plan.CDC.User)
	}
}
