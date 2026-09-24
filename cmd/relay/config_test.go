package main

import (
	"strings"
	"testing"
)

func validYAML() string {
	return `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
}

func TestParseConfig_nameAndCluster(t *testing.T) {
	spec, err := parseConfig([]byte(validYAML()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "example" {
		t.Fatalf("got name %q", spec.Name)
	}
	if spec.Cluster != "prod" {
		t.Fatalf("got cluster %q", spec.Cluster)
	}
}

func TestParseConfig_rejectsHyphenatedName(t *testing.T) {
	raw := strings.Replace(validYAML(), "name: example\n", "name: example-service\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when name has a hyphen")
	}
}

func TestParseConfig_rejectsHyphenatedTable(t *testing.T) {
	raw := strings.Replace(validYAML(), "name: orders\n", "name: order-items\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when table name has a hyphen")
	}
}

func TestParseConfig_rejectsHyphenatedDatabase(t *testing.T) {
	if _, err := parseConfig([]byte(validYAML() + "database:\n  name: shop-db\n")); err == nil {
		t.Fatal("expected error when database name has a hyphen")
	}
}

func TestParseConfig_acceptsUnderscoreColumn(t *testing.T) {
	raw := strings.Replace(validYAML(), "name: id\n", "name: user_id\n", 1)
	if _, err := parseConfig([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfig_rejectsCreateInstanceHyphen(t *testing.T) {
	raw := strings.Replace(validYAML(), "  create: true\n", "  create: true\n  name: shared-rds\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when create Instance name has a hyphen")
	}
}

func TestParseConfig_allowsAttachInstanceHyphen(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: widgets
cluster: prod
instance:
  create: false
  name: shared-rds
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfig_rejectsProjectKey(t *testing.T) {
	raw := strings.Replace(validYAML(), "name: example\n", "project: example\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when identity is project")
	}
}

func TestParseConfig_rejectsPassword(t *testing.T) {
	if _, err := parseConfig([]byte(validYAML() + "password: secret\n")); err == nil {
		t.Fatal("expected error when password is source")
	}
}

func TestParseConfig_rejectsTopicsAsSource(t *testing.T) {
	if _, err := parseConfig([]byte(validYAML() + "topics:\n  - name: x\n")); err == nil {
		t.Fatal("expected error when topics is source")
	}
}

func TestParseConfig_requiresName(t *testing.T) {
	raw := strings.Replace(validYAML(), "name: example\n", "name: \"\"\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when name is empty")
	}
}

func TestParseConfig_requiresCluster(t *testing.T) {
	raw := strings.Replace(validYAML(), "cluster: prod\n", "", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when cluster is missing")
	}
}

func TestParseConfig_requiresKindConfig(t *testing.T) {
	raw := strings.Replace(validYAML(), "kind: Config\n", "kind: Project\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when kind is not Config")
	}
}

func TestParseConfig_requiresAPIVersion(t *testing.T) {
	raw := strings.Replace(validYAML(), "apiVersion: relay/v1\n", "apiVersion: relay/v2\n", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when apiVersion is not relay/v1")
	}
}

func TestParseConfig_requiresTables(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when tables is missing")
	}
}

func TestParseConfig_pkCannotBeNullable(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - name: id
        type: integer
        primary_key: true
        nullable: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when PK column is nullable")
	}
}

func TestParseConfig_requiresPrimaryKey(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - name: amount
        type: numeric
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when Table has no primary key")
	}
}

func TestParseConfig_requiresColumnName(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when column name is missing")
	}
}

func TestParseConfig_attachRequiresInstanceName(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: widgets
cluster: prod
instance:
  create: false
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when attach omits Instance name")
	}
}

func TestParseConfig_instanceNameMax40(t *testing.T) {
	name := strings.Repeat("a", 41)
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
  name: ` + name + `
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when Instance name is longer than 40")
	}
}

func TestParseConfig_nameAsInstanceMax40(t *testing.T) {
	name := strings.Repeat("a", 41)
	raw := `apiVersion: relay/v1
kind: Config
name: ` + name + `
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when Config name used as Instance name is longer than 40")
	}
}

func TestParseConfig_instanceName40Allowed(t *testing.T) {
	name := strings.Repeat("a", 40)
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
  name: ` + name + `
tables:
  - name: orders
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfig_requiresTableName(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when Table name is missing")
	}
}

func TestParseConfig_rejectsUnlistedColumnType(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    columns:
      - name: id
        type: varchar
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when column type is not allowlisted")
	}
}

func TestParseConfig_rejectsTopicNameOverride(t *testing.T) {
	raw := `apiVersion: relay/v1
kind: Config
name: example
cluster: prod
instance:
  create: true
tables:
  - name: orders
    topic: custom.topic
    columns:
      - name: id
        type: serial
        primary_key: true
`
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected error when topic name is declared")
	}
}

func TestParseConfig_databaseNameOptional(t *testing.T) {
	raw := validYAML() + "database:\n  name: shop\n"
	spec, err := parseConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Database.Name != "shop" {
		t.Fatalf("got database %q", spec.Database.Name)
	}
}

func TestParseConfig_rejectsDatabasesList(t *testing.T) {
	if _, err := parseConfig([]byte(validYAML() + "databases:\n  - shop\n")); err == nil {
		t.Fatal("expected error when databases list is source")
	}
}

func TestParseConfig_rejectsCDCSuffix(t *testing.T) {
	for _, extra := range []string{"prefix: acme_cdc\n", "database:\n  name: orders_cdc\n"} {
		if _, err := parseConfig([]byte(validYAML() + extra)); err == nil || !strings.Contains(err.Error(), "_cdc") {
			t.Fatalf("%q: got %v", extra, err)
		}
	}
}

func TestParseConfig_rejectsInvalidCluster(t *testing.T) {
	raw := strings.Replace(validYAML(), "cluster: prod", "cluster: \"prod'; --\"", 1)
	if _, err := parseConfig([]byte(raw)); err == nil {
		t.Fatal("expected invalid cluster error")
	}
}

func TestParseConfig_rejectsReservedNames(t *testing.T) {
	cases := map[string]string{
		"postgres database":  "database:\n  name: postgres\n",
		"rdsadmin database":  "database:\n  name: rdsadmin\n",
		"template database":  "database:\n  name: template1\n",
		"rds_superuser role": "prefix: rds\ndatabase:\n  name: superuser\n",
		"pg_ role":           "prefix: pg\n",
	}
	for name, extra := range cases {
		if _, err := parseConfig([]byte(validYAML() + extra)); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	for _, schema := range []string{"pg_catalog", "information_schema"} {
		raw := strings.Replace(validYAML(), "  - name: orders\n", "  - name: orders\n    schema: "+schema+"\n", 1)
		if _, err := parseConfig([]byte(raw)); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("schema %s: got %v", schema, err)
		}
	}
}
