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
	acmeVars, _, err := writeApplyFiles(configStateDir(root, "acme"), "topic_name = \"acme.public.orders\"\n", "")
	if err != nil {
		t.Fatal(err)
	}
	bravoVars, _, err := writeApplyFiles(configStateDir(root, "bravo"), "topic_name = \"bravo.public.orders\"\n", "")
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
}

func TestTerraformEnv_omitsPassword(t *testing.T) {
	for _, e := range terraformEnv("/tmp/acme") {
		if strings.Contains(strings.ToLower(e), "password") || strings.HasPrefix(e, "TF_VAR_") {
			t.Fatalf("secret in terraform env: %s", e)
		}
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
	if !strings.Contains(out, "sink_control_topic = \"acme.control.iceberg\"") {
		t.Fatalf("missing control topic: %s", out)
	}
	if !strings.Contains(out, "iceberg_table = \"acme_public_orders\"") {
		t.Fatalf("missing iceberg table: %s", out)
	}
	if strings.Contains(out, "example-service_public") {
		t.Fatalf("hyphenated Glue name in tfvars: %s", out)
	}
	if !strings.Contains(out, "secret_name = \"acme\"") {
		t.Fatalf("missing secret_name: %s", out)
	}
	if !strings.Contains(out, "secret_user_key = \"user\"") {
		t.Fatalf("missing secret_user_key: %s", out)
	}
}

func TestRenderTfvars_createPassesMasterSecretARN(t *testing.T) {
	plan, err := planApply(validSpec(), liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	plan.Connection.Secret = "rds!db-acme-AbCdEf"
	plan.Connection.SecretARN = "arn:aws:secretsmanager:us-east-1:000000000000:secret:rds!db-acme-AbCdEf"
	plan.Connection.SecretUserKey = "username"
	out := renderTfvars(plan, clusterEnv{Region: "us-east-1"})
	if strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("password in tfvars: %s", out)
	}
	if !strings.Contains(out, "secret_name = \"rds!db-acme-AbCdEf\"") {
		t.Fatalf("missing secret name: %s", out)
	}
	if !strings.Contains(out, "secret_user_key = \"username\"") {
		t.Fatalf("missing user key: %s", out)
	}
	if !strings.Contains(out, "master_secret_arn = \"arn:aws:secretsmanager:us-east-1:000000000000:secret:rds!db-acme-AbCdEf\"") {
		t.Fatalf("missing master secret arn: %s", out)
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

func TestRenderTfvars_exampleYAMLUsesGlueSafeIcebergName(t *testing.T) {
	spec := validSpec()
	spec.Name = "example-service"
	spec.Tables[0].Columns = []column{
		{Name: "id", Type: "serial", PrimaryKey: true},
		{Name: "amount", Type: "numeric", Nullable: boolPtr(false)},
	}
	plan, err := planApply(spec, liveSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	out := renderTfvars(plan, clusterEnv{Region: "us-east-1"})
	if !strings.Contains(out, "iceberg_table = \"example_service_public_orders\"") {
		t.Fatalf("missing glue-safe iceberg name: %s", out)
	}
	if strings.Contains(out, "example-service_public_orders") {
		t.Fatalf("hyphenated Glue name: %s", out)
	}
	if !strings.Contains(out, "sink_control_topic = \"example-service.control.iceberg\"") {
		t.Fatalf("missing control topic: %s", out)
	}
	if !strings.Contains(out, "{ name = \"amount\", type = \"decimal(38,9)\" }") {
		t.Fatalf("missing iceberg columns: %s", out)
	}
}

func TestParseApplyFlags_requiresFile(t *testing.T) {
	if _, _, err := parseApplyFlags(nil); err == nil {
		t.Fatal("expected error when -f is missing")
	}
}

func TestParseApplyFlags_stateBucketFromEnv(t *testing.T) {
	t.Setenv("RELAY_STATE_BUCKET", "relay-prod-state")
	_, env, err := parseApplyFlags([]string{"-f", "x.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if env.StateBucket != "relay-prod-state" {
		t.Fatalf("got %q", env.StateBucket)
	}
	if env.MigrateState {
		t.Fatal("migrate-state must default false")
	}
}

func TestParseApplyFlags_migrateState(t *testing.T) {
	_, env, err := parseApplyFlags([]string{"-f", "x.yaml", "--migrate-state"})
	if err != nil {
		t.Fatal(err)
	}
	if !env.MigrateState {
		t.Fatal("expected --migrate-state")
	}
}

func TestStateKey_includesClusterAndName(t *testing.T) {
	got := stateKey("prod", "acme")
	if got != "relay/prod/acme/terraform.tfstate" {
		t.Fatalf("got %q", got)
	}
	if stateKey("prod", "acme") == stateKey("prod", "bravo") {
		t.Fatal("configs must not share state key")
	}
	if stateKey("prod", "acme") == stateKey("staging", "acme") {
		t.Fatal("clusters must not share state key")
	}
}

func TestBackendInitArgs_remoteS3(t *testing.T) {
	got := strings.Join(backendInitArgs("relay-prod-state", "relay/prod/acme/terraform.tfstate", "us-east-1"), " ")
	if !strings.Contains(got, "-backend-config=bucket=relay-prod-state") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "-backend-config=key=relay/prod/acme/terraform.tfstate") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "-backend-config=region=us-east-1") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "-backend-config=encrypt=true") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "-backend-config=use_lockfile=true") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "-state=") {
		t.Fatalf("local -state= still present: %s", got)
	}
}

func TestHeadObjectArgs_passesBucketKeyRegion(t *testing.T) {
	got := strings.Join(headObjectArgs("relay-prod-state", "relay/prod/acme/terraform.tfstate", "eu-west-1"), " ")
	if !strings.Contains(got, "s3api head-object --bucket relay-prod-state --key relay/prod/acme/terraform.tfstate") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--region eu-west-1") {
		t.Fatalf("got %q", got)
	}
}

func TestDecideState_emptyIsNew(t *testing.T) {
	action, err := decideState(false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if action != actionInit {
		t.Fatalf("got %v", action)
	}
}

func TestDecideState_localWithoutFlagRefused(t *testing.T) {
	_, err := decideState(true, false, false)
	if err == nil || !strings.Contains(err.Error(), "--migrate-state") {
		t.Fatalf("got %v", err)
	}
}

func TestDecideState_localWithRemoteIgnoresLocal(t *testing.T) {
	action, err := decideState(true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if action != actionInit {
		t.Fatalf("got %v", action)
	}
}

func TestDecideState_migrateUploadsWhenRemoteEmpty(t *testing.T) {
	action, err := decideState(true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if action != actionMigrate {
		t.Fatalf("got %v", action)
	}
}

func TestDecideState_migrateRefusedWhenRemoteExists(t *testing.T) {
	_, err := decideState(true, true, true)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("got %v", err)
	}
}

func TestDecideState_migrateRefusedWhenLocalMissing(t *testing.T) {
	_, err := decideState(false, false, true)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v", err)
	}
}

func TestClusterEnv_validateRequiresStateBucket(t *testing.T) {
	env := validClusterEnv()
	env.StateBucket = ""
	if err := env.validate(false); err == nil || !strings.Contains(err.Error(), "state-bucket") {
		t.Fatalf("got %v", err)
	}
}

func TestClusterEnv_validateCreateRequiresNetwork(t *testing.T) {
	env := validClusterEnv()
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

func validClusterEnv() clusterEnv {
	return clusterEnv{
		Region:            "us-east-1",
		MSKBootstrap:      "b:9098",
		MSKClusterARN:     "arn:msk",
		WarehouseBucket:   "wh",
		StateBucket:       "relay-prod-state",
		GlueDatabase:      "glue",
		DebeziumPluginARN: "arn:d",
		IcebergPluginARN:  "arn:i",
		ConnectRoleARN:    "arn:r",
		ConnectSubnetIDs:  []string{"subnet-1"},
		ConnectSGIds:      []string{"sg-1"},
	}
}
