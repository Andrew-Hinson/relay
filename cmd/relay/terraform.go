package main

import (
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

func writeApplyFiles(stateDir, tfvars, sql string) (tfvarsPath, statePath, sqlPath string, err error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", "", "", err
	}
	tfvarsPath = filepath.Join(stateDir, "terraform.tfvars")
	statePath = filepath.Join(stateDir, "terraform.tfstate")
	sqlPath = filepath.Join(stateDir, "apply.sql")
	if err := os.WriteFile(tfvarsPath, []byte(tfvars), 0644); err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(sqlPath, []byte(sql), 0644); err != nil {
		return "", "", "", err
	}
	return tfvarsPath, statePath, sqlPath, nil
}

func renderTfvars(plan applyPlan, env clusterEnv) string {
	var b strings.Builder
	writeStr(&b, "region", env.Region)
	writeStr(&b, "cluster", plan.Cluster)
	writeStr(&b, "instance_name", plan.Instance.Name)
	writeBool(&b, "instance_create", plan.Instance.Create)
	writeStr(&b, "database_name", plan.Database.Name)
	writeStr(&b, "rds_username", plan.Connection.User)
	writeStr(&b, "msk_bootstrap_servers", env.MSKBootstrap)
	writeStr(&b, "msk_cluster_arn", env.MSKClusterARN)
	writeStr(&b, "warehouse_bucket", env.WarehouseBucket)
	writeStr(&b, "glue_database", env.GlueDatabase)
	writeStr(&b, "debezium_plugin_arn", env.DebeziumPluginARN)
	writeStr(&b, "iceberg_plugin_arn", env.IcebergPluginARN)
	writeStr(&b, "connect_role_arn", env.ConnectRoleARN)
	writeList(&b, "connect_subnet_ids", env.ConnectSubnetIDs)
	writeList(&b, "connect_sg_ids", env.ConnectSGIds)
	writeStr(&b, "vpc_id", env.VPCID)
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

func terraformOutput(dir, statePath, name string) (string, error) {
	cmd := exec.Command("terraform", "output", "-raw", "-state="+statePath, name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
