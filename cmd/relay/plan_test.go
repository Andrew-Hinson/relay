package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

func validSpec() configFile {
	return configFile{
		APIVersion: "relay/v1",
		Kind:       "Config",
		Name:       "acme",
		Cluster:    "prod",
		Instance:   instanceSpec{Create: true},
		Tables: []table{{
			Name:    "orders",
			Columns: []column{{Name: "id", Type: "serial", PrimaryKey: true}},
		}},
	}
}

func TestPlanApply_prefixDefaultsToName(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Prefix != "acme" {
		t.Fatalf("got prefix %q", plan.Prefix)
	}
	if plan.Tables[0].Topic != "acme.public.orders" {
		t.Fatalf("got topic %q", plan.Tables[0].Topic)
	}
}

func TestPlanApply_prefixOverride(t *testing.T) {
	spec := validSpec()
	spec.Prefix = "widgets"
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tables[0].Topic != "widgets.public.orders" {
		t.Fatalf("got topic %q", plan.Tables[0].Topic)
	}
}

func TestPlanApply_schemaOverride(t *testing.T) {
	spec := validSpec()
	spec.Tables[0].Schema = "sales"
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tables[0].Topic != "acme.sales.orders" {
		t.Fatalf("got topic %q", plan.Tables[0].Topic)
	}
	if plan.Tables[0].Iceberg != "acme_sales_orders" {
		t.Fatalf("got iceberg %q", plan.Tables[0].Iceberg)
	}
	if plan.Tables[0].RouteValue != "sales.orders" {
		t.Fatalf("got route %q", plan.Tables[0].RouteValue)
	}
}

func TestPlanApply_icebergNameIsGlueSafe(t *testing.T) {
	spec := validSpec()
	spec.Name = "example"
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tables[0].Iceberg != "example_public_orders" {
		t.Fatalf("got iceberg %q", plan.Tables[0].Iceberg)
	}
	if plan.Sink.ControlTopic != "example.control.iceberg" {
		t.Fatalf("got control topic %q", plan.Sink.ControlTopic)
	}
	if plan.Tables[0].IDColumns != "id" {
		t.Fatalf("got id columns %q", plan.Tables[0].IDColumns)
	}
	if len(plan.Tables[0].Columns) != 1 || plan.Tables[0].Columns[0] != (plannedColumn{Name: "id", Type: "int"}) {
		t.Fatalf("got columns %+v", plan.Tables[0].Columns)
	}
}

func TestPlanApply_instanceNameDefaultsToConfig(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Instance.Name != "acme" || !plan.Instance.Create {
		t.Fatalf("got Instance %+v", plan.Instance)
	}
}

func TestPlanApply_databaseDefaultsToConfig(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Database.Name != "acmedb" || plan.Database.DDL != "CREATE DATABASE acmedb OWNER acme_acmedb" {
		t.Fatalf("got Database %+v", plan.Database)
	}
}

func TestPlanApply_databaseNameOverride(t *testing.T) {
	spec := validSpec()
	spec.Database.Name = "shop"
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Database.Name != "shop" || plan.Database.DDL != "CREATE DATABASE shop OWNER acme_shop" {
		t.Fatalf("got Database %+v", plan.Database)
	}
}

func TestPlanApply_skipsDatabaseDDLWhenLiveHasIt(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{Databases: []string{"acmedb"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Database.DDL != "" {
		t.Fatalf("got Database %+v, want no DDL", plan.Database)
	}
}

func TestPlanApply_printedConnectionIsOwnerNotCDC(t *testing.T) {
	spec := validSpec()
	spec.Instance.Name = "shared-rds"
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Connection.User != "acme_acmedb" {
		t.Fatalf("got user %q", plan.Connection.User)
	}
	if plan.CDC.User != "acme_acmedb_cdc" {
		t.Fatalf("got CDC user %q", plan.CDC.User)
	}
}

func TestPlanApply_oneConnectorPerDatabase(t *testing.T) {
	spec := validSpec()
	spec.Tables = append(spec.Tables, table{
		Name:    "items",
		Columns: []column{{Name: "id", Type: "serial", PrimaryKey: true}},
	})
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Connector.Name != "acme-acmedb-cdc" {
		t.Fatalf("got connector %q", plan.Connector.Name)
	}
	if plan.Connector.TableIncludeList != "public.orders,public.items" {
		t.Fatalf("got include %q", plan.Connector.TableIncludeList)
	}
	if plan.Connector.Publication != "acme_acmedb_cdc" {
		t.Fatalf("got publication %q", plan.Connector.Publication)
	}
	if plan.Connector.PublicationDDL != "CREATE PUBLICATION acme_acmedb_cdc FOR TABLE public.orders, public.items" {
		t.Fatalf("got publication DDL %q", plan.Connector.PublicationDDL)
	}
	if plan.Sink.Name != "acme-acmedb-iceberg" {
		t.Fatalf("got sink %q", plan.Sink.Name)
	}
	if strings.Join(plan.Sink.Topics, ",") != "acme.public.orders,acme.public.items" {
		t.Fatalf("got sink topics %v", plan.Sink.Topics)
	}
}

func TestPlanApply_kafkaDefaults(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kafka.Partitions != 3 || plan.Kafka.Replicas != 3 || plan.Kafka.MinInsyncReplicas != 2 {
		t.Fatalf("got kafka %+v", plan.Kafka)
	}
}

func TestPlanApply_partitionsMayRaise(t *testing.T) {
	spec := validSpec()
	spec.Kafka.Partitions = intPtr(6)
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kafka.Partitions != 6 {
		t.Fatalf("got partitions %d", plan.Kafka.Partitions)
	}
}

func TestPlanApply_partitionsBelowDefaultFails(t *testing.T) {
	spec := validSpec()
	spec.Kafka.Partitions = intPtr(2)
	if _, err := planApply(spec, liveSnapshot{}); err == nil {
		t.Fatal("expected error when partitions is below default")
	}
}

func TestPlanApply_replicasBelowDefaultFails(t *testing.T) {
	spec := validSpec()
	spec.Kafka.Replicas = intPtr(1)
	if _, err := planApply(spec, liveSnapshot{}); err == nil {
		t.Fatal("expected error when replicas is below default")
	}
}

func TestPlanApply_generatesDDL(t *testing.T) {
	spec := validSpec()
	spec.Tables[0].Columns = []column{
		{Name: "id", Type: "serial", PrimaryKey: true},
		{Name: "user_id", Type: "integer", Nullable: boolPtr(false)},
	}
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE TABLE IF NOT EXISTS public.orders (\n  id SERIAL PRIMARY KEY,\n  user_id INTEGER NOT NULL\n)"
	if plan.Tables[0].DDL != want {
		t.Fatalf("got DDL:\n%s\nwant:\n%s", plan.Tables[0].DDL, want)
	}
}

func TestPlanApply_failsWhenLiveTableDoesNotMatchYAML(t *testing.T) {
	live := liveSnapshot{
		Tables: []liveTable{{
			Database: "acmedb",
			Schema:   "public",
			Name:     "orders",
			Columns: []liveColumn{
				{Name: "id", Type: "serial", PrimaryKey: true},
				{Name: "amount", Type: "numeric", Nullable: true},
			},
		}},
	}
	if _, err := planApply(validSpec(), live); err == nil {
		t.Fatal("expected error when live Table does not match YAML DDL")
	}
}

func TestPlanApply_reApplySucceedsWhenLiveMatchesYAML(t *testing.T) {
	live := liveSnapshot{
		Databases: []string{"acmedb"},
		Tables: []liveTable{{
			Database: "acmedb",
			Schema:   "public",
			Name:     "orders",
			Columns:  []liveColumn{{Name: "id", Type: "serial", PrimaryKey: true}},
		}},
	}
	plan, err := planApply(validSpec(), live)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.Tables[0].DDL, "CREATE TABLE IF NOT EXISTS") {
		t.Fatalf("got DDL %q", plan.Tables[0].DDL)
	}
	if strings.Contains(plan.Tables[0].DDL, "ALTER") {
		t.Fatal("re-Apply must not ALTER")
	}
	if plan.Connector.PublicationDDL != "CREATE PUBLICATION acme_acmedb_cdc FOR TABLE public.orders" {
		t.Fatalf("got publication DDL %q", plan.Connector.PublicationDDL)
	}
}

func TestPlanApply_skipsPublicationWhenLiveHasIt(t *testing.T) {
	live := liveSnapshot{
		Databases: []string{"acmedb"},
		Tables: []liveTable{{
			Database: "acmedb",
			Schema:   "public",
			Name:     "orders",
			Columns:  []liveColumn{{Name: "id", Type: "serial", PrimaryKey: true}},
		}},
		Publications: []livePublication{{Name: "acme_acmedb_cdc", Tables: []string{"public.orders"}}},
	}
	plan, err := planApply(validSpec(), live)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Connector.PublicationDDL != "" {
		t.Fatalf("got publication DDL %q", plan.Connector.PublicationDDL)
	}
	if len(plan.Connector.PublicationAdds) != 0 {
		t.Fatalf("got adds %v", plan.Connector.PublicationAdds)
	}
}

func TestPlanApply_addsNewTableToLivePublication(t *testing.T) {
	spec := validSpec()
	spec.Tables = append(spec.Tables, table{
		Name:    "items",
		Columns: []column{{Name: "id", Type: "serial", PrimaryKey: true}},
	})
	live := liveSnapshot{
		Databases:    []string{"acmedb"},
		Publications: []livePublication{{Name: "acme_acmedb_cdc", Tables: []string{"public.orders"}}},
	}
	plan, err := planApply(spec, live)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Connector.PublicationDDL != "" {
		t.Fatalf("got publication DDL %q", plan.Connector.PublicationDDL)
	}
	if len(plan.Connector.PublicationAdds) != 1 || plan.Connector.PublicationAdds[0] != "ALTER PUBLICATION acme_acmedb_cdc ADD TABLE public.items" {
		t.Fatalf("got adds %v", plan.Connector.PublicationAdds)
	}
}

func TestPlanApply_exampleYAML(t *testing.T) {
	tfDir, err := findTFDir()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(tfDir), "example", "example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := parseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "example" {
		t.Fatalf("got name %q", spec.Name)
	}
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tables[0].Topic != "example.public.orders" {
		t.Fatalf("got topic %q", plan.Tables[0].Topic)
	}
	if plan.Tables[0].Iceberg != "example_public_orders" {
		t.Fatalf("got iceberg %q", plan.Tables[0].Iceberg)
	}
	if plan.Sink.ControlTopic != "example.control.iceberg" {
		t.Fatalf("got control topic %q", plan.Sink.ControlTopic)
	}
	if plan.Connector.TableIncludeList != "public.orders" {
		t.Fatalf("got include %q", plan.Connector.TableIncludeList)
	}
	if plan.Database.Name != "exampledb" || plan.Database.DDL != "CREATE DATABASE exampledb OWNER example_exampledb" {
		t.Fatalf("got Database %+v", plan.Database)
	}
	if plan.Instance.Name != "example" || !plan.Instance.Create {
		t.Fatalf("got Instance %+v", plan.Instance)
	}
}

func TestFormatConnection_omitsPassword(t *testing.T) {
	out := formatConnection(plannedConnection{
		Endpoint: "db.example",
		Database: "acme",
		User:     "acme",
	})
	if strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("password leaked: %s", out)
	}
	if strings.Contains(out, "secret:") {
		t.Fatalf("secret leaked: %s", out)
	}
	if !strings.Contains(out, "auth: iam") {
		t.Fatalf("got %s", out)
	}
}

func boolPtr(v bool) *bool { return &v }
