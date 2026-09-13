package main

import (
	"errors"
	"flag"
	"os"
	"strings"
)

type clusterEnv struct {
	Region            string
	MSKBootstrap      string
	MSKClusterARN     string
	WarehouseBucket   string
	GlueDatabase      string
	DebeziumPluginARN string
	IcebergPluginARN  string
	ConnectRoleARN    string
	ConnectSubnetIDs  []string
	ConnectSGIds      []string
	SubnetIDs         []string
	RDSSGIds          []string
	RDSInstanceClass  string
	RDSEngineVersion  string
	StateBucket       string
	MigrateState      bool
}

func parseApplyFlags(args []string) (file string, env clusterEnv, err error) {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fileFlag := fs.String("f", "", "Config YAML")
	region := fs.String("region", envOr("RELAY_REGION", ""), "AWS region")
	msk := fs.String("msk-bootstrap-servers", envOr("RELAY_MSK_BOOTSTRAP_SERVERS", ""), "MSK bootstrap servers")
	mskARN := fs.String("msk-cluster-arn", envOr("RELAY_MSK_CLUSTER_ARN", ""), "MSK cluster ARN")
	bucket := fs.String("warehouse-bucket", envOr("RELAY_WAREHOUSE_BUCKET", ""), "Warehouse S3 bucket")
	glue := fs.String("glue-database", envOr("RELAY_GLUE_DATABASE", ""), "Glue database")
	debezium := fs.String("debezium-plugin-arn", envOr("RELAY_DEBEZIUM_PLUGIN_ARN", ""), "MSK Connect Debezium plugin ARN")
	iceberg := fs.String("iceberg-plugin-arn", envOr("RELAY_ICEBERG_PLUGIN_ARN", ""), "MSK Connect Iceberg plugin ARN")
	role := fs.String("connect-role-arn", envOr("RELAY_CONNECT_ROLE_ARN", ""), "MSK Connect service role ARN")
	connectSubnets := fs.String("connect-subnet-ids", envOr("RELAY_CONNECT_SUBNET_IDS", ""), "MSK Connect subnet IDs (comma-separated)")
	connectSGs := fs.String("connect-sg-ids", envOr("RELAY_CONNECT_SG_IDS", ""), "MSK Connect security group IDs (comma-separated)")
	subnets := fs.String("subnet-ids", envOr("RELAY_SUBNET_IDS", ""), "RDS subnet IDs (comma-separated)")
	rdsSGs := fs.String("rds-sg-ids", envOr("RELAY_RDS_SG_IDS", ""), "RDS security group IDs (comma-separated)")
	class := fs.String("rds-instance-class", envOr("RELAY_RDS_INSTANCE_CLASS", "db.t3.medium"), "RDS instance class")
	engine := fs.String("rds-engine-version", envOr("RELAY_RDS_ENGINE_VERSION", "16"), "RDS engine version")
	stateBucket := fs.String("state-bucket", envOr("RELAY_STATE_BUCKET", ""), "Cluster Terraform state bucket")
	migrateState := fs.Bool("migrate-state", false, "one-shot copy of local terraform.tfstate to the Cluster state bucket")
	if err := fs.Parse(args); err != nil {
		return "", clusterEnv{}, err
	}
	if *fileFlag == "" {
		return "", clusterEnv{}, errors.New("usage: relay apply -f <config.yaml>")
	}
	env = clusterEnv{
		Region:            *region,
		MSKBootstrap:      *msk,
		MSKClusterARN:     *mskARN,
		WarehouseBucket:   *bucket,
		GlueDatabase:      *glue,
		DebeziumPluginARN: *debezium,
		IcebergPluginARN:  *iceberg,
		ConnectRoleARN:    *role,
		ConnectSubnetIDs:  csv(*connectSubnets),
		ConnectSGIds:      csv(*connectSGs),
		SubnetIDs:         csv(*subnets),
		RDSSGIds:          csv(*rdsSGs),
		RDSInstanceClass:  *class,
		RDSEngineVersion:  *engine,
		StateBucket:       *stateBucket,
		MigrateState:      *migrateState,
	}
	return *fileFlag, env, nil
}

func (e clusterEnv) validate(createInstance bool) error {
	required := []struct {
		name, val string
	}{
		{"--region / RELAY_REGION", e.Region},
		{"--msk-bootstrap-servers / RELAY_MSK_BOOTSTRAP_SERVERS", e.MSKBootstrap},
		{"--msk-cluster-arn / RELAY_MSK_CLUSTER_ARN", e.MSKClusterARN},
		{"--warehouse-bucket / RELAY_WAREHOUSE_BUCKET", e.WarehouseBucket},
		{"--state-bucket / RELAY_STATE_BUCKET", e.StateBucket},
		{"--glue-database / RELAY_GLUE_DATABASE", e.GlueDatabase},
		{"--debezium-plugin-arn / RELAY_DEBEZIUM_PLUGIN_ARN", e.DebeziumPluginARN},
		{"--iceberg-plugin-arn / RELAY_ICEBERG_PLUGIN_ARN", e.IcebergPluginARN},
		{"--connect-role-arn / RELAY_CONNECT_ROLE_ARN", e.ConnectRoleARN},
	}
	for _, r := range required {
		if r.val == "" {
			return errors.New(r.name + " is required")
		}
	}
	if len(e.ConnectSubnetIDs) == 0 {
		return errors.New("--connect-subnet-ids / RELAY_CONNECT_SUBNET_IDS is required")
	}
	if len(e.ConnectSGIds) == 0 {
		return errors.New("--connect-sg-ids / RELAY_CONNECT_SG_IDS is required")
	}
	if !createInstance {
		return nil
	}
	if len(e.SubnetIDs) == 0 {
		return errors.New("--subnet-ids / RELAY_SUBNET_IDS is required when instance.create")
	}
	if len(e.RDSSGIds) == 0 {
		return errors.New("--rds-sg-ids / RELAY_RDS_SG_IDS is required when instance.create")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func csv(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
