package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exampleProfile(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "example", "cluster.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseProfile_exampleIsValid(t *testing.T) {
	p, err := parseProfile(exampleProfile(t), "prod")
	if err != nil {
		t.Fatal(err)
	}
	var env clusterEnv
	env.fillFrom(p)
	env.applyDefaults()
	if err := env.validate(true); err != nil {
		t.Fatalf("example profile is incomplete: %v", err)
	}
	if err := env.checkCluster("prod"); err != nil {
		t.Fatal(err)
	}
	if err := env.checkARNs(); err != nil {
		t.Fatal(err)
	}
}

func TestParseProfile_rejects(t *testing.T) {
	base := string(exampleProfile(t))
	cases := map[string]struct {
		raw, cluster, want string
	}{
		"other cluster":   {base, "staging", "does not match"},
		"wrong kind":      {strings.Replace(base, "kind: Cluster", "kind: Config", 1), "prod", "kind"},
		"unknown key":     {base + "password: hunter2\n", "prod", "password"},
		"unquoted digits": {strings.Replace(base, `account: "111122223333"`, "account: 12", 1), "prod", "12-digit"},
		"missing region":  {strings.Replace(base, "region: us-east-1\n", "", 1), "prod", "region"},
	}
	for name, c := range cases {
		if _, err := parseProfile([]byte(c.raw), c.cluster); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: got %v, want %q", name, err, c.want)
		}
	}
}

func TestProfilePaths_repoBeforeUserConfig(t *testing.T) {
	got := profilePaths("/repo", "/home/u/.config", "prod")
	want := []string{"/repo/clusters/prod.yaml", "/home/u/.config/relay/clusters/prod.yaml"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
	if got := profilePaths("", "/home/u/.config", "prod"); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestFillFrom_flagsAndEnvWin(t *testing.T) {
	p, err := parseProfile(exampleProfile(t), "prod")
	if err != nil {
		t.Fatal(err)
	}
	env := clusterEnv{StateBucket: "override-state", ConnectSGIds: []string{"sg-override"}}
	env.fillFrom(p)
	if env.StateBucket != "override-state" {
		t.Fatalf("got %q; override must win", env.StateBucket)
	}
	if strings.Join(env.ConnectSGIds, ",") != "sg-override" {
		t.Fatalf("got %v; override must win", env.ConnectSGIds)
	}
	if env.WarehouseBucket != "relay-warehouse" || env.Account != "111122223333" {
		t.Fatalf("profile did not fill: %+v", env)
	}
}

func TestParseApplyFlags_rdsDefaultsAfterProfile(t *testing.T) {
	_, env, err := parseApplyFlags([]string{"x.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if env.RDSInstanceClass != "" {
		t.Fatalf("flag default %q would shadow the profile", env.RDSInstanceClass)
	}
	env.fillFrom(clusterProfile{RDS: profileRDS{InstanceClass: "db.r6g.large"}})
	env.applyDefaults()
	if env.RDSInstanceClass != "db.r6g.large" || env.RDSEngineVersion != defaultRDSEngineVersion {
		t.Fatalf("got %q / %q", env.RDSInstanceClass, env.RDSEngineVersion)
	}
}

func TestCheckARNs_catchesOtherAccountOrRegion(t *testing.T) {
	p, err := parseProfile(exampleProfile(t), "prod")
	if err != nil {
		t.Fatal(err)
	}
	var env clusterEnv
	env.fillFrom(p)
	env.ConnectRoleARN = "arn:aws:iam::999999999999:role/relay-connect"
	if err := env.checkARNs(); err == nil || !strings.Contains(err.Error(), "role_arn") {
		t.Fatalf("got %v", err)
	}
	env.fillFrom(p)
	env.ConnectRoleARN = p.Connect.RoleARN
	env.IcebergPluginARN = strings.Replace(p.Connect.IcebergPluginARN, "us-east-1", "eu-west-1", 1)
	if err := env.checkARNs(); err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("got %v", err)
	}
	if err := (clusterEnv{MSKClusterARN: "anything"}).checkARNs(); err != nil {
		t.Fatalf("no profile account must skip the check, got %v", err)
	}
}

func TestCallerAccountArgs(t *testing.T) {
	got := strings.Join(callerAccountArgs("us-east-1"), " ")
	if got != "sts get-caller-identity --query Account --output text --region us-east-1" {
		t.Fatalf("got %q", got)
	}
}

func TestFindProfile_loadsFromRepoClusters(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"tf", "clusters"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "clusters", "prod.yaml"), exampleProfile(t), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(root)
	p, path, err := findProfile("prod")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "clusters", "prod.yaml") || p.Name != "prod" {
		t.Fatalf("got %q %+v", path, p)
	}
	if _, path, err := findProfile("staging"); err != nil || path != "" {
		t.Fatalf("missing profile must be allowed, got %q %v", path, err)
	}
}
