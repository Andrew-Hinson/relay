package main

import (
	"fmt"
	"strings"
)

type liveSnapshot struct {
	Databases []string
	Tables    []liveTable
}

type liveTable struct {
	Database string
	Schema   string
	Name     string
	Columns  []liveColumn
}

type liveColumn struct {
	Name       string
	Type       string
	Nullable   bool
	PrimaryKey bool
}

type applyPlan struct {
	Name       string
	Cluster    string
	Prefix     string
	Instance   plannedInstance
	Database   plannedDatabase
	Connection plannedConnection
	Kafka      plannedKafka
	Tables     []plannedTable
	Connector  plannedConnector
	Sink       plannedSink
}

type plannedInstance struct {
	Name   string
	Create bool
}

type plannedDatabase struct {
	Name string
	DDL  string
}

type plannedConnection struct {
	Endpoint string
	Database string
	User     string
	Secret   string
}

type plannedKafka struct {
	Partitions        int
	Replicas          int
	MinInsyncReplicas int
}

type plannedTable struct {
	Name     string
	Schema   string
	Database string
	Topic    string
	Iceberg  string
	DDL      string
}

type plannedConnector struct {
	Name             string
	Class            string
	Database         string
	TableIncludeList string
	TopicPrefix      string
	Publication      string
}

type plannedSink struct {
	Name   string
	Topics []string
}

func planApply(spec configFile, live liveSnapshot) (applyPlan, error) {
	var plan applyPlan
	prefix := spec.Prefix
	if prefix == "" {
		prefix = spec.Name
	}
	instName := instanceName(spec)
	dbName := databaseName(spec)
	partitions := defaultPartitions
	if spec.Kafka.Partitions != nil {
		partitions = *spec.Kafka.Partitions
	}
	if partitions < defaultPartitions {
		return plan, fmt.Errorf("partitions %d is below default %d", partitions, defaultPartitions)
	}
	replicas := defaultReplicas
	if spec.Kafka.Replicas != nil {
		replicas = *spec.Kafka.Replicas
	}
	if replicas < defaultReplicas {
		return plan, fmt.Errorf("replicas %d is below default %d", replicas, defaultReplicas)
	}
	minISR := defaultMinISR
	if spec.Kafka.MinInsyncReplicas != nil {
		minISR = *spec.Kafka.MinInsyncReplicas
	}
	if minISR < defaultMinISR {
		return plan, fmt.Errorf("min.insync.replicas %d is below default %d", minISR, defaultMinISR)
	}

	plan.Name = spec.Name
	plan.Cluster = spec.Cluster
	plan.Prefix = prefix
	plan.Instance = plannedInstance{Name: instName, Create: spec.Instance.Create}
	plan.Database = plannedDatabase{Name: dbName}
	if !liveHasDatabase(live, dbName) {
		plan.Database.DDL = "CREATE DATABASE " + dbName
	}
	plan.Connection = plannedConnection{
		Endpoint: instName,
		Database: dbName,
		User:     instName,
		Secret:   instName,
	}
	plan.Kafka = plannedKafka{Partitions: partitions, Replicas: replicas, MinInsyncReplicas: minISR}

	var include []string
	var topics []string
	for _, t := range spec.Tables {
		schema := t.Schema
		if schema == "" {
			schema = "public"
		}
		if liveTbl, ok := findLiveTable(live, dbName, schema, t.Name); ok {
			if !tableMatchesYAML(t.Columns, liveTbl.Columns) {
				return plan, fmt.Errorf("table %s.%s does not match YAML DDL", schema, t.Name)
			}
		}
		topic := prefix + "." + schema + "." + t.Name
		plan.Tables = append(plan.Tables, plannedTable{
			Name:     t.Name,
			Schema:   schema,
			Database: dbName,
			Topic:    topic,
			Iceberg:  prefix + "_" + schema + "_" + t.Name,
			DDL:      tableDDL(schema, t.Name, t.Columns),
		})
		include = append(include, schema+"."+t.Name)
		topics = append(topics, topic)
	}

	connectorName := prefix + "-" + dbName + "-cdc"
	plan.Connector = plannedConnector{
		Name:             connectorName,
		Class:            debeziumPostgresClass,
		Database:         dbName,
		TableIncludeList: strings.Join(include, ","),
		TopicPrefix:      prefix,
		Publication:      strings.ReplaceAll(connectorName, "-", "_"),
	}
	plan.Sink = plannedSink{
		Name:   prefix + "-" + dbName + "-iceberg",
		Topics: topics,
	}
	return plan, nil
}

func tableDDL(schema, name string, cols []column) string {
	var pks []string
	for _, col := range cols {
		if col.PrimaryKey {
			pks = append(pks, col.Name)
		}
	}
	inlinePK := len(pks) == 1
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS ")
	b.WriteString(schema)
	b.WriteByte('.')
	b.WriteString(name)
	b.WriteString(" (\n")
	for i, col := range cols {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  ")
		b.WriteString(col.Name)
		b.WriteByte(' ')
		b.WriteString(sqlColumnTypes[col.Type])
		if col.PrimaryKey && inlinePK {
			b.WriteString(" PRIMARY KEY")
		} else if !columnNullable(col) {
			b.WriteString(" NOT NULL")
		}
	}
	if !inlinePK {
		b.WriteString(",\n  PRIMARY KEY (")
		b.WriteString(strings.Join(pks, ", "))
		b.WriteString(")")
	}
	b.WriteString("\n)")
	return b.String()
}

func liveHasDatabase(live liveSnapshot, name string) bool {
	for _, db := range live.Databases {
		if db == name {
			return true
		}
	}
	return false
}

func findLiveTable(live liveSnapshot, database, schema, name string) (liveTable, bool) {
	for _, tbl := range live.Tables {
		if tbl.Database == database && tbl.Schema == schema && tbl.Name == name {
			return tbl, true
		}
	}
	return liveTable{}, false
}

func tableMatchesYAML(want []column, live []liveColumn) bool {
	if len(want) != len(live) {
		return false
	}
	byName := make(map[string]liveColumn, len(live))
	for _, col := range live {
		byName[col.Name] = col
	}
	for _, col := range want {
		got, ok := byName[col.Name]
		if !ok {
			return false
		}
		if got.Type != col.Type || got.Nullable != columnNullable(col) || got.PrimaryKey != col.PrimaryKey {
			return false
		}
	}
	return true
}

func columnNullable(col column) bool {
	if col.PrimaryKey {
		return false
	}
	if col.Nullable == nil {
		return true
	}
	return *col.Nullable
}

func renderApplySQL(plan applyPlan) string {
	var b strings.Builder
	if plan.Database.DDL != "" {
		b.WriteString(plan.Database.DDL)
		b.WriteByte('\n')
	}
	for _, tbl := range plan.Tables {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(tbl.DDL)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatConnection(conn plannedConnection) string {
	return "endpoint: " + conn.Endpoint + "\n" +
		"database: " + conn.Database + "\n" +
		"user: " + conn.User + "\n" +
		"secret: " + conn.Secret + "\n"
}

var sqlColumnTypes = map[string]string{
	"integer":     "INTEGER",
	"bigint":      "BIGINT",
	"text":        "TEXT",
	"numeric":     "NUMERIC",
	"boolean":     "BOOLEAN",
	"timestamptz": "TIMESTAMPTZ",
	"serial":      "SERIAL",
}

const (
	debeziumPostgresClass = "io.debezium.connector.postgresql.PostgresConnector"
	defaultPartitions     = 3
	defaultReplicas       = 3
	defaultMinISR         = 2
	postgresPort          = "5432"
)
