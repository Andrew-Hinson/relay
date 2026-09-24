package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type configFile struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Name       string       `yaml:"name"`
	Cluster    string       `yaml:"cluster"`
	Prefix     string       `yaml:"prefix"`
	Instance   instanceSpec `yaml:"instance"`
	Database   databaseSpec `yaml:"database"`
	Tables     []table      `yaml:"tables"`
	Kafka      kafkaSpec    `yaml:"kafka"`
}

type instanceSpec struct {
	Create bool   `yaml:"create"`
	Name   string `yaml:"name"`
}

type databaseSpec struct {
	Name string `yaml:"name"`
}

type table struct {
	Name    string   `yaml:"name"`
	Schema  string   `yaml:"schema"`
	Columns []column `yaml:"columns"`
}

type column struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Nullable   *bool  `yaml:"nullable"`
	PrimaryKey bool   `yaml:"primary_key"`
}

type kafkaSpec struct {
	Partitions        *int `yaml:"partitions"`
	Replicas          *int `yaml:"replicas"`
	MinInsyncReplicas *int `yaml:"min.insync.replicas"`
}

func parseConfig(raw []byte) (configFile, error) {
	var spec configFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return spec, err
	}
	if spec.APIVersion != "relay/v1" {
		return spec, errors.New("apiVersion must be relay/v1")
	}
	if spec.Kind != "Config" {
		return spec, errors.New("kind must be Config")
	}
	if spec.Name == "" {
		return spec, errors.New("name is required")
	}
	if !rdsUserIdent(spec.Name) {
		return spec, fmt.Errorf("name %q is not a valid identifier", spec.Name)
	}
	if spec.Cluster == "" {
		return spec, errors.New("cluster is required")
	}
	if !clusterIdent(spec.Cluster) {
		return spec, fmt.Errorf("cluster %q is not a valid MSK cluster name", spec.Cluster)
	}
	if spec.Prefix != "" && !sqlIdent(spec.Prefix) {
		return spec, fmt.Errorf("prefix %q is not a valid identifier", spec.Prefix)
	}
	if spec.Database.Name != "" && !sqlIdent(spec.Database.Name) {
		return spec, fmt.Errorf("database %q is not a valid identifier", spec.Database.Name)
	}
	// Owner roles are {prefix}_{database}; a _cdc suffix would land in the CDC role namespace.
	if strings.HasSuffix(spec.Prefix, cdcSuffix) || strings.HasSuffix(spec.Database.Name, cdcSuffix) {
		return spec, fmt.Errorf("prefix and database must not end in %q", cdcSuffix)
	}
	if err := checkReservedNames(spec); err != nil {
		return spec, err
	}
	if !spec.Instance.Create && spec.Instance.Name == "" {
		return spec, errors.New("attach instance requires name")
	}
	if spec.Instance.Name != "" {
		ok := rdsUserIdent(spec.Instance.Name)
		if !spec.Instance.Create {
			ok = rdsIdent(spec.Instance.Name)
		}
		if !ok {
			return spec, fmt.Errorf("instance name %q is not a valid identifier", spec.Instance.Name)
		}
	}
	instName := instanceName(spec)
	if len(instName) > 40 {
		return spec, errors.New("instance name must be at most 40 characters")
	}
	if len(spec.Tables) == 0 {
		return spec, errors.New("tables is required")
	}
	for _, tbl := range spec.Tables {
		if tbl.Name == "" {
			return spec, errors.New("table name is required")
		}
		if !sqlIdent(tbl.Name) {
			return spec, fmt.Errorf("table %q is not a valid identifier", tbl.Name)
		}
		if tbl.Schema != "" && !sqlIdent(tbl.Schema) {
			return spec, fmt.Errorf("schema %q is not a valid identifier", tbl.Schema)
		}
		if reservedSchema(tbl.Schema) {
			return spec, fmt.Errorf("schema %q is reserved", tbl.Schema)
		}
		hasPK := false
		for _, col := range tbl.Columns {
			if col.Name == "" {
				return spec, errors.New("column name is required")
			}
			if !sqlIdent(col.Name) {
				return spec, fmt.Errorf("column %q is not a valid identifier", col.Name)
			}
			if !allowedColumnTypes[col.Type] {
				return spec, fmt.Errorf("column type %q is not allowed", col.Type)
			}
			if col.PrimaryKey {
				hasPK = true
				if col.Nullable != nil && *col.Nullable {
					return spec, errors.New("primary key cannot be nullable")
				}
			}
		}
		if !hasPK {
			return spec, errors.New("table requires a primary key")
		}
	}
	return spec, nil
}

func instanceName(spec configFile) string {
	if spec.Instance.Name != "" {
		return spec.Instance.Name
	}
	return spec.Name
}

func databaseName(spec configFile) string {
	if spec.Database.Name != "" {
		return spec.Database.Name
	}
	return spec.Name + "db"
}

// checkReservedNames keeps a Config off Postgres and RDS system databases and roles,
// so claiming them never depends on the stamp check or a Postgres permission error.
func checkReservedNames(spec configFile) error {
	db := databaseName(spec)
	if reservedDatabases[db] || strings.HasPrefix(db, "template") {
		return fmt.Errorf("database %q is reserved", db)
	}
	owner, cdc := configRoles(spec)
	for _, role := range []string{owner, cdc} {
		if strings.HasPrefix(role, "pg_") || strings.HasPrefix(role, "rds") {
			return fmt.Errorf("role %q is reserved (pg_ and rds prefixes belong to Postgres and RDS)", role)
		}
	}
	return nil
}

func reservedSchema(s string) bool {
	return s == "information_schema" || strings.HasPrefix(s, "pg_")
}

var reservedDatabases = map[string]bool{
	"postgres": true,
	"rdsadmin": true,
}

func sqlIdent(s string) bool {
	return identCharset(s, true, false)
}

func rdsUserIdent(s string) bool {
	return identCharset(s, false, false)
}

func rdsIdent(s string) bool {
	return identCharset(s, false, true)
}

// clusterIdent matches MSK cluster names: letters, digits, hyphens, leading letter.
func clusterIdent(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-'):
		default:
			return false
		}
	}
	return s != ""
}

func identCharset(s string, underscore, hyphen bool) bool {
	if s == "" {
		return false
	}
	prevHyphen := false
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			prevHyphen = false
		case i > 0 && r >= '0' && r <= '9':
			prevHyphen = false
		case i > 0 && underscore && r == '_':
			prevHyphen = false
		case i > 0 && hyphen && r == '-' && !prevHyphen:
			prevHyphen = true
		default:
			return false
		}
	}
	return !prevHyphen
}

var allowedColumnTypes = map[string]bool{
	"integer":     true,
	"bigint":      true,
	"text":        true,
	"numeric":     true,
	"boolean":     true,
	"timestamptz": true,
	"serial":      true,
}
