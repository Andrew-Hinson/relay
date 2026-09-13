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

func TestDescribeDBInstanceArgs_passesRegion(t *testing.T) {
	got := strings.Join(describeDBInstanceArgs("shared-rds", "eu-west-1"), " ")
	if !strings.Contains(got, "--db-instance-identifier shared-rds") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}
