package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func findTFDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		cand := filepath.Join(dir, "tf")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("tf/ not found; run from the repo")
		}
		dir = parent
	}
}

func configStateDir(root, name string) string {
	return filepath.Join(root, ".relay", name)
}

func writeApplyFiles(stateDir, tfvars, sql string) (tfvarsPath, sqlPath string, err error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", "", err
	}
	tfvarsPath = filepath.Join(stateDir, "terraform.tfvars")
	sqlPath = filepath.Join(stateDir, "apply.sql")
	if err := os.WriteFile(tfvarsPath, []byte(tfvars), 0644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(sqlPath, []byte(sql), 0644); err != nil {
		return "", "", err
	}
	return tfvarsPath, sqlPath, nil
}

func localStatePath(stateDir string) string {
	return filepath.Join(stateDir, "terraform.tfstate")
}

func stateKey(cluster, name string) string {
	return "relay/" + cluster + "/" + name + "/terraform.tfstate"
}

func terraformEnv(stateDir string) []string {
	return []string{"TF_DATA_DIR=" + filepath.Join(stateDir, ".terraform")}
}

func backendInitArgs(bucket, key, region string) []string {
	return []string{
		"init",
		"-input=false",
		"-reconfigure",
		"-backend-config=bucket=" + bucket,
		"-backend-config=key=" + key,
		"-backend-config=region=" + region,
		"-backend-config=encrypt=true",
		"-backend-config=use_lockfile=true",
	}
}

func headObjectArgs(bucket, key, region string) []string {
	return awsRegionArgs(region, "s3api", "head-object", "--bucket", bucket, "--key", key)
}

func remoteStateExists(bucket, key, region string) (bool, error) {
	out, err := exec.Command("aws", headObjectArgs(bucket, key, region)...).CombinedOutput()
	if err == nil {
		return true, nil
	}
	s := string(out)
	if strings.Contains(s, "404") || strings.Contains(s, "NotFound") || strings.Contains(s, "Not Found") {
		return false, nil
	}
	return false, fmt.Errorf("state s3://%s/%s: %w", bucket, key, err)
}

type stateAction int

const (
	actionInit stateAction = iota
	actionMigrate
)

func decideState(hasLocal, hasRemote, migrate bool) (stateAction, error) {
	switch {
	case migrate && !hasLocal:
		return 0, fmt.Errorf("--migrate-state set but local terraform.tfstate is missing")
	case migrate && hasRemote:
		return 0, fmt.Errorf("remote state already exists; --migrate-state refused")
	case migrate:
		return actionMigrate, nil
	case hasLocal && !hasRemote:
		return 0, fmt.Errorf("local state exists; pass --migrate-state once to copy it to the Cluster state bucket")
	default:
		return actionInit, nil
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func initApplyBackend(tfDir, stateDir string, env clusterEnv, cluster, name string) error {
	key := stateKey(cluster, name)
	local := localStatePath(stateDir)
	hasLocal := fileExists(local)
	var hasRemote bool
	if hasLocal || env.MigrateState {
		var err error
		hasRemote, err = remoteStateExists(env.StateBucket, key, env.Region)
		if err != nil {
			return err
		}
	}
	action, err := decideState(hasLocal, hasRemote, env.MigrateState)
	if err != nil {
		return fmt.Errorf("%w (s3://%s/%s)", err, env.StateBucket, key)
	}
	tfEnv := terraformEnv(stateDir)
	if err := runTerraform(tfDir, tfEnv, backendInitArgs(env.StateBucket, key, env.Region)...); err != nil {
		return err
	}
	if action != actionMigrate {
		return nil
	}
	if err := runTerraform(tfDir, tfEnv, "state", "push", "-force", local); err != nil {
		return err
	}
	return os.Rename(local, local+".migrated")
}

func renderTfvars(plan applyPlan, env clusterEnv) string {
	var b strings.Builder
	writeStr(&b, "region", env.Region)
	writeStr(&b, "instance_name", plan.Instance.Name)
	writeBool(&b, "instance_create", plan.Instance.Create)
	writeStr(&b, "database_name", plan.Database.Name)
	writeStr(&b, "rds_username", plan.Instance.Username)
	writeStr(&b, "cdc_user", plan.CDC.User)
	writeStr(&b, "msk_bootstrap_servers", env.MSKBootstrap)
	writeStr(&b, "msk_cluster_arn", env.MSKClusterARN)
	writeStr(&b, "warehouse_bucket", env.WarehouseBucket)
	writeStr(&b, "glue_database", env.GlueDatabase)
	writeStr(&b, "debezium_plugin_arn", env.DebeziumPluginARN)
	writeStr(&b, "iceberg_plugin_arn", env.IcebergPluginARN)
	writeStr(&b, "connect_role_arn", env.ConnectRoleARN)
	writeStr(&b, "connect_source_boundary_arn", env.ConnectSourceBoundaryARN)
	writeStr(&b, "connect_worker_policy_arn", env.ConnectWorkerPolicyARN)
	writeList(&b, "connect_subnet_ids", env.ConnectSubnetIDs)
	writeList(&b, "connect_sg_ids", env.ConnectSGIds)
	writeList(&b, "subnet_ids", env.SubnetIDs)
	writeList(&b, "rds_sg_ids", env.RDSSGIds)
	writeStr(&b, "rds_instance_class", env.RDSInstanceClass)
	writeStr(&b, "rds_engine_version", env.RDSEngineVersion)
	writeNum(&b, "partitions", plan.Kafka.Partitions)
	writeNum(&b, "replicas", plan.Kafka.Replicas)
	writeNum(&b, "min_insync_replicas", plan.Kafka.MinInsyncReplicas)
	writeStr(&b, "connector_name", plan.Connector.Name)
	writeStr(&b, "connector_class", plan.Connector.Class)
	writeStr(&b, "connector_database", plan.Connector.Database)
	writeStr(&b, "table_include_list", plan.Connector.TableIncludeList)
	writeStr(&b, "connector_topic_prefix", plan.Connector.TopicPrefix)
	writeStr(&b, "publication_name", plan.Connector.Publication)
	writeStr(&b, "sink_name", plan.Sink.Name)
	writeStr(&b, "sink_control_topic", plan.Sink.ControlTopic)
	writeStr(&b, "connector_hostname", plan.Connection.Endpoint)
	writeList(&b, "sink_topics", plan.Sink.Topics)
	b.WriteString("tables = [\n")
	for _, t := range plan.Tables {
		b.WriteString("  {\n")
		writeStrIndent(&b, "    ", "topic_name", t.Topic)
		writeStrIndent(&b, "    ", "iceberg_table", t.Iceberg)
		writeStrIndent(&b, "    ", "route_value", t.RouteValue)
		writeStrIndent(&b, "    ", "id_columns", t.IDColumns)
		b.WriteString("    columns = [\n")
		for _, c := range t.Columns {
			b.WriteString("      { name = ")
			b.WriteString(strconv.Quote(c.Name))
			b.WriteString(", type = ")
			b.WriteString(strconv.Quote(c.Type))
			b.WriteString(" },\n")
		}
		b.WriteString("    ]\n")
		b.WriteString("  },\n")
	}
	b.WriteString("]\n")
	return b.String()
}

func writeStrIndent(b *strings.Builder, indent, key, v string) {
	b.WriteString(indent)
	writeStr(b, key, v)
}

func writeBool(b *strings.Builder, key string, v bool) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.FormatBool(v))
	b.WriteByte('\n')
}

func writeStr(b *strings.Builder, key, v string) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.Quote(v))
	b.WriteByte('\n')
}

func writeNum(b *strings.Builder, key string, v int) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.Itoa(v))
	b.WriteByte('\n')
}

func writeList(b *strings.Builder, key string, vs []string) {
	b.WriteString(key)
	b.WriteString(" = [")
	for i, v := range vs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(v))
	}
	b.WriteString("]\n")
}

// planArgs saves the plan so Apply executes exactly what was reviewed.
// -detailed-exitcode exits 2 when the plan has changes.
func planArgs(tfvarsPath, planPath string, targets ...string) []string {
	args := []string{"plan", "-input=false", "-detailed-exitcode", "-var-file=" + tfvarsPath, "-out=" + planPath}
	for _, t := range targets {
		args = append(args, "-target="+t)
	}
	return args
}

// applyPlanArgs applies a saved plan; Terraform does not prompt for saved plans.
func applyPlanArgs(planPath string) []string {
	return []string{"apply", "-input=false", planPath}
}

// terraformPlan runs plan and reports whether the saved plan has changes.
func terraformPlan(dir string, extraEnv []string, args ...string) (bool, error) {
	err := runTerraform(dir, extraEnv, args...)
	var ee *exec.ExitError
	switch {
	case err == nil:
		return false, nil
	case errors.As(err, &ee) && ee.ExitCode() == 2:
		return true, nil
	default:
		return false, err
	}
}

func runTerraform(dir string, extraEnv []string, args ...string) error {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

func terraformOutput(dir, stateDir, name string) (string, error) {
	cmd := exec.Command("terraform", "output", "-raw", name)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), terraformEnv(stateDir)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
