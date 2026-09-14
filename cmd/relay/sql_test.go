package main

import (
	"strings"
	"testing"
)

func TestGrantSQL_scopesConfigRole(t *testing.T) {
	role, db := "acme_acmedb_cdc", "acmedb"
	if grantReplicationSQL(role) != "GRANT rds_replication TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantReplicationSQL(role))
	}
	if grantDatabaseSQL(role, db) != "GRANT CONNECT, CREATE ON DATABASE acmedb TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantDatabaseSQL(role, db))
	}
	if revokePublicConnectSQL(db) != "REVOKE CONNECT ON DATABASE acmedb FROM PUBLIC" {
		t.Fatalf("got %q", revokePublicConnectSQL(db))
	}
	if grantSchemaSQL(role, "public") != "GRANT USAGE, CREATE ON SCHEMA public TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantSchemaSQL(role, "public"))
	}
	if alterTableOwnerSQL("public", "orders", role) != "ALTER TABLE public.orders OWNER TO acme_acmedb_cdc" {
		t.Fatalf("got %q", alterTableOwnerSQL("public", "orders", role))
	}
	if alterDatabaseOwnerSQL(db, role) != "ALTER DATABASE acmedb OWNER TO acme_acmedb_cdc" {
		t.Fatalf("got %q", alterDatabaseOwnerSQL(db, role))
	}
}

func TestRenderApplySQL_omitsPassword(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	out := renderApplySQL(plan)
	if strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("password in apply.sql: %s", out)
	}
	if strings.Contains(out, "CREATE ROLE") {
		t.Fatal("role password DDL must not land in apply.sql")
	}
	if !strings.Contains(out, "CREATE DATABASE acmedb OWNER acme_acmedb_cdc") {
		t.Fatalf("got %s", out)
	}
}

func TestConfigRoleName_matchesPublication(t *testing.T) {
	if got := configRoleName("acme", "acmedb"); got != "acme_acmedb_cdc" {
		t.Fatalf("got %q", got)
	}
	if got := configSecretName("prod", "acme"); got != "relay/prod/acme/cdc" {
		t.Fatalf("got %q", got)
	}
}

func TestQuoteLiteral_doublesQuotes(t *testing.T) {
	if got := quoteLiteral(`a'b`); got != `'a''b'` {
		t.Fatalf("got %q", got)
	}
}
