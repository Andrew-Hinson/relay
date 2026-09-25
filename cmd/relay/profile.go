package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// clusterProfile is the committed description of a shared Cluster, selected by a
// Config's `cluster:`. It holds ARNs and IDs only; Relay authenticates with IAM.
type clusterProfile struct {
	APIVersion  string         `yaml:"apiVersion"`
	Kind        string         `yaml:"kind"`
	Name        string         `yaml:"name"`
	Account     string         `yaml:"account"`
	Region      string         `yaml:"region"`
	MSK         profileMSK     `yaml:"msk"`
	Warehouse   profileStorage `yaml:"warehouse"`
	StateBucket string         `yaml:"state_bucket"`
	Connect     profileConnect `yaml:"connect"`
	RDS         profileRDS     `yaml:"rds"`
}

type profileMSK struct {
	ClusterARN       string `yaml:"cluster_arn"`
	BootstrapServers string `yaml:"bootstrap_servers"`
}

type profileStorage struct {
	Bucket       string `yaml:"bucket"`
	GlueDatabase string `yaml:"glue_database"`
}

type profileConnect struct {
	DebeziumPluginARN string   `yaml:"debezium_plugin_arn"`
	IcebergPluginARN  string   `yaml:"iceberg_plugin_arn"`
	RoleARN           string   `yaml:"role_arn"`
	SourceBoundaryARN string   `yaml:"source_boundary_arn"`
	WorkerPolicyARN   string   `yaml:"worker_policy_arn"`
	SubnetIDs         []string `yaml:"subnet_ids"`
	SGIDs             []string `yaml:"sg_ids"`
}

type profileRDS struct {
	SubnetIDs     []string `yaml:"subnet_ids"`
	SGIDs         []string `yaml:"sg_ids"`
	InstanceClass string   `yaml:"instance_class"`
	EngineVersion string   `yaml:"engine_version"`
}

func parseProfile(raw []byte, cluster string) (clusterProfile, error) {
	var p clusterProfile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	switch {
	case p.APIVersion != "relay/v1":
		return p, errors.New("apiVersion must be relay/v1")
	case p.Kind != "Cluster":
		return p, errors.New("kind must be Cluster")
	case p.Name != cluster:
		return p, fmt.Errorf("name %q does not match Config cluster %q", p.Name, cluster)
	case !awsAccountID(p.Account):
		return p, fmt.Errorf("account %q must be a 12-digit AWS account ID (quote it in YAML)", p.Account)
	case p.Region == "":
		return p, errors.New("region is required")
	}
	return p, nil
}

func awsAccountID(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// profilePaths lists where the profile for a Cluster may live, in lookup order:
// the repo's clusters/ directory, then the user's config directory.
func profilePaths(repoRoot, userConfig, cluster string) []string {
	var paths []string
	if repoRoot != "" {
		paths = append(paths, filepath.Join(repoRoot, "clusters", cluster+".yaml"))
	}
	if userConfig != "" {
		paths = append(paths, filepath.Join(userConfig, "relay", "clusters", cluster+".yaml"))
	}
	return paths
}

// findProfile loads the first profile found for cluster. A missing profile is not
// an error: flags and RELAY_* can still describe the Cluster.
func findProfile(cluster string) (clusterProfile, string, error) {
	var repoRoot, userConfig string
	if tfDir, err := findTFDir(); err == nil {
		repoRoot = filepath.Dir(tfDir)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		userConfig = dir
	}
	for _, path := range profilePaths(repoRoot, userConfig, cluster) {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return clusterProfile{}, "", err
		}
		p, err := parseProfile(raw, cluster)
		if err != nil {
			return clusterProfile{}, "", fmt.Errorf("%s: %w", path, err)
		}
		return p, path, nil
	}
	return clusterProfile{}, "", nil
}

// fillFrom sets every field flags and RELAY_* left empty from the profile,
// so flags win over env, and env wins over the profile.
func (e *clusterEnv) fillFrom(p clusterProfile) {
	fill := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	fillList := func(dst *[]string, v []string) {
		if len(*dst) == 0 {
			*dst = v
		}
	}
	e.Account = p.Account
	fill(&e.Region, p.Region)
	fill(&e.MSKClusterARN, p.MSK.ClusterARN)
	fill(&e.MSKBootstrap, p.MSK.BootstrapServers)
	fill(&e.WarehouseBucket, p.Warehouse.Bucket)
	fill(&e.GlueDatabase, p.Warehouse.GlueDatabase)
	fill(&e.StateBucket, p.StateBucket)
	fill(&e.DebeziumPluginARN, p.Connect.DebeziumPluginARN)
	fill(&e.IcebergPluginARN, p.Connect.IcebergPluginARN)
	fill(&e.ConnectRoleARN, p.Connect.RoleARN)
	fill(&e.ConnectSourceBoundaryARN, p.Connect.SourceBoundaryARN)
	fill(&e.ConnectWorkerPolicyARN, p.Connect.WorkerPolicyARN)
	fillList(&e.ConnectSubnetIDs, p.Connect.SubnetIDs)
	fillList(&e.ConnectSGIds, p.Connect.SGIDs)
	fillList(&e.SubnetIDs, p.RDS.SubnetIDs)
	fillList(&e.RDSSGIds, p.RDS.SGIDs)
	fill(&e.RDSInstanceClass, p.RDS.InstanceClass)
	fill(&e.RDSEngineVersion, p.RDS.EngineVersion)
}

// checkARNs requires every ARN to live in the profile's account and, for regional
// services, its region, catching values copied from another Cluster.
func (e clusterEnv) checkARNs() error {
	if e.Account == "" {
		return nil
	}
	for _, a := range []struct{ name, arn string }{
		{"msk cluster_arn", e.MSKClusterARN},
		{"connect debezium_plugin_arn", e.DebeziumPluginARN},
		{"connect iceberg_plugin_arn", e.IcebergPluginARN},
		{"connect role_arn", e.ConnectRoleARN},
		{"connect source_boundary_arn", e.ConnectSourceBoundaryARN},
		{"connect worker_policy_arn", e.ConnectWorkerPolicyARN},
	} {
		parts := strings.SplitN(a.arn, ":", 6)
		if len(parts) != 6 || parts[0] != "arn" {
			return fmt.Errorf("%s %q is not an ARN", a.name, a.arn)
		}
		if parts[4] != e.Account {
			return fmt.Errorf("%s %q is not in account %s", a.name, a.arn, e.Account)
		}
		if parts[3] != "" && parts[3] != e.Region {
			return fmt.Errorf("%s %q is not in region %s", a.name, a.arn, e.Region)
		}
	}
	return nil
}

func callerAccountArgs(region string) []string {
	return awsRegionArgs(region, "sts", "get-caller-identity", "--query", "Account", "--output", "text")
}

// checkCallerAccount refuses to run with credentials for a different account
// than the Cluster profile names.
func checkCallerAccount(account, region string) error {
	out, err := exec.Command("aws", callerAccountArgs(region)...).Output()
	if err != nil {
		return fmt.Errorf("aws credentials: %s", awsErrMsg(out, err))
	}
	if got := strings.TrimSpace(string(out)); got != account {
		return fmt.Errorf("aws credentials are for account %s, but Cluster profile is account %s", got, account)
	}
	return nil
}
