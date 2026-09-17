package main

import (
	"strings"
	"testing"
)

func TestGrantSQL_scopesOwnerAndCDC(t *testing.T) {
	owner, cdc, db := "acme_acmedb", "acme_acmedb_cdc", "acmedb"
	if grantReplicationSQL(cdc) != "GRANT rds_replication TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantReplicationSQL(cdc))
	}
	if grantRDSIAMSQL("relay") != "GRANT rds_iam TO relay" {
		t.Fatalf("got %q", grantRDSIAMSQL("relay"))
	}
	if grantRDSIAMSQL(owner) != "GRANT rds_iam TO acme_acmedb" {
		t.Fatalf("got %q", grantRDSIAMSQL(owner))
	}
	if grantRDSIAMSQL(cdc) != "GRANT rds_iam TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantRDSIAMSQL(cdc))
	}
	if grantOwnerDatabaseSQL(owner, db) != "GRANT CONNECT, CREATE ON DATABASE acmedb TO acme_acmedb" {
		t.Fatalf("got %q", grantOwnerDatabaseSQL(owner, db))
	}
	if grantCDCDatabaseSQL(cdc, db) != "GRANT CONNECT ON DATABASE acmedb TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantCDCDatabaseSQL(cdc, db))
	}
	if strings.Contains(grantCDCDatabaseSQL(cdc, db), "CREATE") {
		t.Fatal("CDC must not have CREATE on the database")
	}
	if revokeCreateDatabaseSQL(cdc, db) != "REVOKE CREATE ON DATABASE acmedb FROM acme_acmedb_cdc" {
		t.Fatalf("got %q", revokeCreateDatabaseSQL(cdc, db))
	}
	if revokePublicConnectSQL(db) != "REVOKE CONNECT ON DATABASE acmedb FROM PUBLIC" {
		t.Fatalf("got %q", revokePublicConnectSQL(db))
	}
	if grantOwnerSchemaSQL(owner, "public") != "GRANT USAGE, CREATE ON SCHEMA public TO acme_acmedb" {
		t.Fatalf("got %q", grantOwnerSchemaSQL(owner, "public"))
	}
	if grantCDCSchemaSQL(cdc, "public") != "GRANT USAGE ON SCHEMA public TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantCDCSchemaSQL(cdc, "public"))
	}
	if strings.Contains(grantCDCSchemaSQL(cdc, "public"), "CREATE") {
		t.Fatal("CDC must not have CREATE on the schema")
	}
	if revokeCreateSchemaSQL(cdc, "public") != "REVOKE CREATE ON SCHEMA public FROM acme_acmedb_cdc" {
		t.Fatalf("got %q", revokeCreateSchemaSQL(cdc, "public"))
	}
	if grantSelectSQL("public", "orders", cdc) != "GRANT SELECT ON TABLE public.orders TO acme_acmedb_cdc" {
		t.Fatalf("got %q", grantSelectSQL("public", "orders", cdc))
	}
	if alterDefaultPrivilegesSQL(owner, "public", cdc) != "ALTER DEFAULT PRIVILEGES FOR ROLE acme_acmedb IN SCHEMA public GRANT SELECT ON TABLES TO acme_acmedb_cdc" {
		t.Fatalf("got %q", alterDefaultPrivilegesSQL(owner, "public", cdc))
	}
	if alterTableOwnerSQL("public", "orders", owner) != "ALTER TABLE public.orders OWNER TO acme_acmedb" {
		t.Fatalf("got %q", alterTableOwnerSQL("public", "orders", owner))
	}
	if alterDatabaseOwnerSQL(db, owner) != "ALTER DATABASE acmedb OWNER TO acme_acmedb" {
		t.Fatalf("got %q", alterDatabaseOwnerSQL(db, owner))
	}
	if alterPublicationOwnerSQL("acme_acmedb_cdc", owner) != "ALTER PUBLICATION acme_acmedb_cdc OWNER TO acme_acmedb" {
		t.Fatalf("got %q", alterPublicationOwnerSQL("acme_acmedb_cdc", owner))
	}
}

func TestPublicationSQL_listsYAMLTables(t *testing.T) {
	tables := []plannedTable{
		{Schema: "public", Name: "orders"},
		{Schema: "public", Name: "items"},
	}
	got := createPublicationSQL("acme_acmedb_cdc", tables)
	if got != "CREATE PUBLICATION acme_acmedb_cdc FOR TABLE public.orders, public.items" {
		t.Fatalf("got %q", got)
	}
	if alterPublicationAddSQL("acme_acmedb_cdc", "sales", "orders") != "ALTER PUBLICATION acme_acmedb_cdc ADD TABLE sales.orders" {
		t.Fatalf("got %q", alterPublicationAddSQL("acme_acmedb_cdc", "sales", "orders"))
	}
}

func TestPostgresURL_requiresSSL(t *testing.T) {
	got := postgresURL(instanceLogin{Host: "db.example", User: "relay", Password: "token"}, "postgres")
	if !strings.Contains(got, "sslmode=require") {
		t.Fatalf("got %q", got)
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
	if !strings.Contains(out, "CREATE DATABASE acmedb OWNER acme_acmedb") {
		t.Fatalf("got %s", out)
	}
	if strings.Contains(out, "OWNER acme_acmedb_cdc") {
		t.Fatal("CDC must not own the database")
	}
	if !strings.Contains(out, "CREATE PUBLICATION acme_acmedb_cdc FOR TABLE public.orders") {
		t.Fatalf("got %s", out)
	}
	if strings.Contains(out, "rds_replication") {
		t.Fatal("replication grant must not land in apply.sql")
	}
	if strings.Contains(out, "rds_iam") {
		t.Fatal("rds_iam grant must not land in apply.sql")
	}
	if strings.Contains(out, "rds_iam") {
		t.Fatal("rds_iam grant must not land in apply.sql")
	}
}

func TestRoleNames_ownerAndCDC(t *testing.T) {
	if got := ownerRoleName("acme", "acmedb"); got != "acme_acmedb" {
		t.Fatalf("got %q", got)
	}
	if got := cdcRoleName("acme", "acmedb"); got != "acme_acmedb_cdc" {
		t.Fatalf("got %q", got)
	}
}
