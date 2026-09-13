package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindTFDir_findsTF(t *testing.T) {
	root := t.TempDir()
	tfDir := filepath.Join(root, "tf")
	if err := os.MkdirAll(tfDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	got, err := findTFDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != tfDir {
		t.Fatalf("got %q, want %q", got, tfDir)
	}
}

func TestConfigStateDir_twoConfigsDoNotShareState(t *testing.T) {
	root := t.TempDir()
	acme := configStateDir(root, "acme")
	bravo := configStateDir(root, "bravo")
	if acme == bravo {
		t.Fatal("configs must not share Apply state")
	}
	if acme != filepath.Join(root, ".relay", "acme") {
		t.Fatalf("got %q", acme)
	}
}

func TestWriteApplyFiles_twoConfigsDoNotClobberTfvars(t *testing.T) {
	root := t.TempDir()
	acmeVars, acmeState, _, err := writeApplyFiles(configStateDir(root, "acme"), "topic_name = \"acme.public.orders\"\n", "")
	if err != nil {
		t.Fatal(err)
	}
	bravoVars, bravoState, _, err := writeApplyFiles(configStateDir(root, "bravo"), "topic_name = \"bravo.public.orders\"\n", "")
	if err != nil {
		t.Fatal(err)
	}
	acmeRaw, err := os.ReadFile(acmeVars)
	if err != nil {
		t.Fatal(err)
	}
	if string(acmeRaw) != "topic_name = \"acme.public.orders\"\n" {
		t.Fatalf("acme tfvars clobbered: %s", acmeRaw)
	}
	bravoRaw, err := os.ReadFile(bravoVars)
	if err != nil {
		t.Fatal(err)
	}
	if string(bravoRaw) != "topic_name = \"bravo.public.orders\"\n" {
		t.Fatalf("bravo tfvars clobbered: %s", bravoRaw)
	}
	if acmeState == bravoState {
		t.Fatal("state paths must differ")
	}
}

func TestRenderTfvars_omitsPassword(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	out := renderTfvars(plan, clusterEnv{Region: "us-east-1"})
	if strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("password in tfvars: %s", out)
	}
	if !strings.Contains(out, "connector_name = \"acme-acmedb-cdc\"") {
		t.Fatalf("missing connector: %s", out)
	}
}

func TestRenderTfvars_scopesConnectIAMByPrefix(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	out := renderTfvars(plan, clusterEnv{
		Region:         "us-east-1",
		MSKClusterARN:  "arn:aws:kafka:us-east-1:000000000000:cluster/prod/00000000-0000-0000-0000-000000000000-0",
		ConnectRoleARN: "arn:aws:iam::000000000000:role/relay-connect",
	})
	if strings.Contains(out, "principal") {
		t.Fatalf("kafka User principal still in tfvars: %s", out)
	}
	if !strings.Contains(out, "connector_topic_prefix = \"acme\"") {
		t.Fatalf("missing topic prefix: %s", out)
	}
	if !strings.Contains(out, "connect_role_arn = \"arn:aws:iam::000000000000:role/relay-connect\"") {
		t.Fatalf("missing connect role: %s", out)
	}
	if !strings.Contains(out, "msk_cluster_arn = \"arn:aws:kafka:us-east-1:000000000000:cluster/prod/00000000-0000-0000-0000-000000000000-0\"") {
		t.Fatalf("missing cluster arn: %s", out)
	}
}

func TestParseApplyFlags_requiresFile(t *testing.T) {
	if _, _, err := parseApplyFlags(nil); err == nil {
		t.Fatal("expected error when -f is missing")
	}
}

func TestClusterEnv_validateCreateRequiresNetwork(t *testing.T) {
	env := clusterEnv{
		Region:            "us-east-1",
		MSKBootstrap:      "b:9098",
		MSKClusterARN:     "arn:msk",
		WarehouseBucket:   "wh",
		GlueDatabase:      "glue",
		DebeziumPluginARN: "arn:d",
		IcebergPluginARN:  "arn:i",
		ConnectRoleARN:    "arn:r",
		ConnectSubnetIDs:  []string{"subnet-1"},
		ConnectSGIds:      []string{"sg-1"},
	}
	if err := env.validate(true); err == nil {
		t.Fatal("expected error when RDS network is missing")
	}
	env.VPCID = "vpc-1"
	env.SubnetIDs = []string{"subnet-1"}
	env.RDSSGIds = []string{"sg-2"}
	if err := env.validate(true); err != nil {
		t.Fatal(err)
	}
}
