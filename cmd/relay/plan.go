package main

import (
	"fmt"
	"strings"
)

type liveSnapshot struct {
	Databases    []string
	Tables       []liveTable
	Publications []livePublication
	// Relay ownership stamps on existing objects, "" when unstamped.
	DatabaseStamps map[string]string
	RoleStamps     map[string]string
}

type livePublication struct {
	Name   string
	Tables []string
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
	Prefix     string
	Stamp      string
	Instance   plannedInstance
	Database   plannedDatabase
	Connection plannedConnection
	CDC        plannedCDC
	Kafka      plannedKafka
	Tables     []plannedTable
	Connector  plannedConnector
	Sink       plannedSink
}

type plannedInstance struct {
	Name     string
	Create   bool
	Username string
}

type plannedDatabase struct {
	Name string
	DDL  string
}

type plannedConnection struct {
	Endpoint string
	Database string
	User     string
}

type plannedCDC struct {
	User string
}

type plannedKafka struct {
	Partitions        int
	Replicas          int
	MinInsyncReplicas int
}

type plannedColumn struct {
	Name string
	Type string
}

type plannedTable struct {
	Name       string
	Schema     string
	Database   string
	Topic      string
	Iceberg    string
	RouteValue string
	IDColumns  string
	Columns    []plannedColumn
	DDL        string
}

type plannedConnector struct {
	Name             string
	Class            string
	Database         string
	TableIncludeList string
	TopicPrefix      string
	Publication      string
	PublicationDDL   string
	PublicationAdds  []string
	PublicationOwner string
}

type plannedSink struct {
	Name         string
	Topics       []string
	ControlTopic string
}

func planApply(spec configFile, live liveSnapshot) (applyPlan, error) {
	var plan applyPlan
	prefix := configPrefix(spec)
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
	plan.Prefix = prefix
	plan.Stamp = ownershipStamp(spec)
	owner := ownerRoleName(prefix, dbName)
	cdc := cdcRoleName(prefix, dbName)
	plan.Instance = plannedInstance{Name: instName, Create: spec.Instance.Create}
	plan.Database = plannedDatabase{Name: dbName}
	if !liveHasDatabase(live, dbName) {
		plan.Database.DDL = "CREATE DATABASE " + dbName + " OWNER " + owner
	}
	plan.Connection = plannedConnection{
		Endpoint: instName,
		Database: dbName,
		User:     owner,
	}
	plan.CDC = plannedCDC{User: cdc}
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
			Name:       t.Name,
			Schema:     schema,
			Database:   dbName,
			Topic:      topic,
			Iceberg:    sanitizeGlueName(prefix + "_" + schema + "_" + t.Name),
			RouteValue: schema + "." + t.Name,
			IDColumns:  idColumns(t.Columns),
			Columns:    icebergColumns(t.Columns),
			DDL:        tableDDL(schema, t.Name, t.Columns),
		})
		include = append(include, schema+"."+t.Name)
		topics = append(topics, topic)
	}

	connectorName := prefix + "-" + dbName + "-cdc"
	pubName := strings.ReplaceAll(connectorName, "-", "_")
	plan.Connector = plannedConnector{
		Name:             connectorName,
		Class:            debeziumPostgresClass,
		Database:         dbName,
		TableIncludeList: strings.Join(include, ","),
		TopicPrefix:      prefix,
		Publication:      pubName,
		PublicationOwner: alterPublicationOwnerSQL(pubName, owner),
	}
	planPublication(live, &plan)
	plan.Sink = plannedSink{
		Name:         prefix + "-" + dbName + "-iceberg",
		Topics:       topics,
		ControlTopic: prefix + ".control.iceberg",
	}
	return plan, nil
}

func configPrefix(spec configFile) string {
	if spec.Prefix != "" {
		return spec.Prefix
	}
	return spec.Name
}

func ownerRoleName(prefix, database string) string {
	return strings.ReplaceAll(prefix+"-"+database, "-", "_")
}

func cdcRoleName(prefix, database string) string {
	return ownerRoleName(prefix, database) + cdcSuffix
}

func configRoles(spec configFile) (owner, cdc string) {
	prefix, db := configPrefix(spec), databaseName(spec)
	return ownerRoleName(prefix, db), cdcRoleName(prefix, db)
}

// ownershipStamp marks Postgres roles and databases as belonging to one Config.
func ownershipStamp(spec configFile) string {
	return "relay:" + spec.Cluster + "/" + spec.Name
}

// checkOwnership refuses existing roles or databases stamped by another Config.
// Unstamped objects predate stamping and are claimed only with adopt.
func checkOwnership(spec configFile, live liveSnapshot, adopt bool) error {
	want := ownershipStamp(spec)
	owner, cdc := configRoles(spec)
	check := func(kind, name string, stamps map[string]string, exists bool) error {
		if !exists {
			return nil
		}
		got := stamps[name]
		switch {
		case got == want:
			return nil
		case got == "" && adopt:
			return nil
		case got == "":
			return fmt.Errorf("%s %s exists without a Relay stamp; pass --adopt once if this Config created it", kind, name)
		default:
			return fmt.Errorf("%s %s belongs to %s, not %s", kind, name, got, want)
		}
	}
	db := databaseName(spec)
	if err := check("database", db, live.DatabaseStamps, liveHasDatabase(live, db)); err != nil {
		return err
	}
	for _, role := range []string{owner, cdc} {
		_, exists := live.RoleStamps[role]
		if err := check("role", role, live.RoleStamps, exists); err != nil {
			return err
		}
	}
	return nil
}

func planPublication(live liveSnapshot, plan *applyPlan) {
	name := plan.Connector.Publication
	livePub, ok := findLivePublication(live, name)
	if !ok {
		plan.Connector.PublicationDDL = createPublicationSQL(name, plan.Tables)
		return
	}
	have := map[string]bool{}
	for _, rel := range livePub.Tables {
		have[rel] = true
	}
	for _, tbl := range plan.Tables {
		rel := tbl.Schema + "." + tbl.Name
		if !have[rel] {
			plan.Connector.PublicationAdds = append(plan.Connector.PublicationAdds, alterPublicationAddSQL(name, tbl.Schema, tbl.Name))
		}
	}
}

func sanitizeGlueName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '_'
		}
	}, name)
}

func idColumns(cols []column) string {
	var pks []string
	for _, col := range cols {
		if col.PrimaryKey {
			pks = append(pks, col.Name)
		}
	}
	return strings.Join(pks, ",")
}

func icebergColumns(cols []column) []plannedColumn {
	out := make([]plannedColumn, len(cols))
	for i, col := range cols {
		out[i] = plannedColumn{Name: col.Name, Type: icebergColumnTypes[col.Type]}
	}
	return out
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

func findLivePublication(live liveSnapshot, name string) (livePublication, bool) {
	for _, pub := range live.Publications {
		if pub.Name == name {
			return pub, true
		}
	}
	return livePublication{}, false
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
	if plan.Connector.PublicationDDL != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(plan.Connector.PublicationDDL)
		b.WriteByte('\n')
	}
	for _, add := range plan.Connector.PublicationAdds {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(add)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatConnection(conn plannedConnection) string {
	return "endpoint: " + conn.Endpoint + "\n" +
		"database: " + conn.Database + "\n" +
		"user: " + conn.User + "\n" +
		"auth: iam\n"
}

type planLine struct {
	Kind string
	Name string
}

type planDiff struct {
	Create   []planLine
	Teardown []planLine
}

func diffPlan(plan applyPlan, live liveSnapshot, instancePresent bool) planDiff {
	var d planDiff
	if plan.Instance.Create && !instancePresent {
		d.Create = append(d.Create, planLine{"instance", plan.Instance.Name})
	}
	if plan.Database.DDL != "" {
		d.Create = append(d.Create, planLine{"database", plan.Database.Name})
	}
	wanted := make(map[string]plannedTable, len(plan.Tables))
	for _, tbl := range plan.Tables {
		key := tbl.Schema + "." + tbl.Name
		wanted[key] = tbl
		if _, ok := findLiveTable(live, tbl.Database, tbl.Schema, tbl.Name); ok {
			continue
		}
		d.Create = append(d.Create,
			planLine{"table", key},
			planLine{"topic", tbl.Topic},
			planLine{"iceberg", tbl.Iceberg},
		)
	}
	for _, lt := range live.Tables {
		if lt.Database != plan.Database.Name {
			continue
		}
		key := lt.Schema + "." + lt.Name
		if _, ok := wanted[key]; ok {
			continue
		}
		d.Teardown = append(d.Teardown,
			planLine{"table", key},
			planLine{"topic", plan.Prefix + "." + lt.Schema + "." + lt.Name},
			planLine{"iceberg", sanitizeGlueName(plan.Prefix + "_" + lt.Schema + "_" + lt.Name)},
		)
	}
	if plan.Connector.PublicationDDL != "" {
		d.Create = append(d.Create,
			planLine{"publication", plan.Connector.Publication},
			planLine{"connector", plan.Connector.Name},
			planLine{"sink", plan.Sink.Name},
		)
	}
	return d
}

func formatPlanDiff(d planDiff) string {
	if len(d.Create) == 0 && len(d.Teardown) == 0 {
		return "No changes.\n"
	}
	var b strings.Builder
	if len(d.Create) > 0 {
		b.WriteString("create\n")
		for _, l := range d.Create {
			b.WriteString("  + ")
			b.WriteString(l.Kind)
			b.WriteByte(' ')
			b.WriteString(l.Name)
			b.WriteByte('\n')
		}
	}
	if len(d.Teardown) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("teardown\n")
		for _, l := range d.Teardown {
			b.WriteString("  - ")
			b.WriteString(l.Kind)
			b.WriteByte(' ')
			b.WriteString(l.Name)
			b.WriteByte('\n')
		}
	}
	return b.String()
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

var icebergColumnTypes = map[string]string{
	"integer":     "int",
	"bigint":      "bigint",
	"text":        "string",
	"numeric":     "decimal(38,9)",
	"boolean":     "boolean",
	"timestamptz": "timestamp",
	"serial":      "int",
}

const (
	debeziumPostgresClass = "io.debezium.connector.postgresql.PostgresConnector"
	defaultPartitions     = 3
	defaultReplicas       = 3
	defaultMinISR         = 2
	postgresPort          = "5432"
	defaultRDSUser        = "relay"
	cdcSuffix             = "_cdc"
)
