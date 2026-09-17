package main

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

type instanceLogin struct {
	Host     string
	User     string
	Password string
}

func inspectLive(login instanceLogin, database string) (liveSnapshot, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, postgresURL(login, "postgres"))
	if err != nil {
		return liveSnapshot{}, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT datname FROM pg_database WHERE datistemplate = false`)
	if err != nil {
		return liveSnapshot{}, err
	}
	defer rows.Close()
	var live liveSnapshot
	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return liveSnapshot{}, err
		}
		dbs = append(dbs, name)
	}
	if err := rows.Err(); err != nil {
		return liveSnapshot{}, err
	}
	live.Databases = dbs
	if !liveHasDatabase(live, database) {
		return live, nil
	}
	tables, pubs, err := inspectDatabase(ctx, login, database)
	if err != nil {
		return liveSnapshot{}, err
	}
	live.Tables = tables
	live.Publications = pubs
	return live, nil
}

func inspectDatabase(ctx context.Context, login instanceLogin, database string) ([]liveTable, []livePublication, error) {
	conn, err := pgx.Connect(ctx, postgresURL(login, database))
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
SELECT n.nspname, c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')`)
	if err != nil {
		return nil, nil, err
	}
	var found []liveTable
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			rows.Close()
			return nil, nil, err
		}
		found = append(found, liveTable{Database: database, Schema: schema, Name: name})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	for i := range found {
		cols, err := inspectColumns(ctx, conn, found[i].Schema, found[i].Name)
		if err != nil {
			return nil, nil, err
		}
		found[i].Columns = cols
	}
	pubs, err := inspectPublications(ctx, conn)
	if err != nil {
		return nil, nil, err
	}
	return found, pubs, nil
}

func inspectPublications(ctx context.Context, conn *pgx.Conn) ([]livePublication, error) {
	rows, err := conn.Query(ctx, `
SELECT p.pubname, n.nspname, c.relname
FROM pg_publication p
LEFT JOIN pg_publication_rel pr ON pr.prpubid = p.oid
LEFT JOIN pg_class c ON c.oid = pr.prrelid
LEFT JOIN pg_namespace n ON n.oid = c.relnamespace`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]*livePublication{}
	var order []string
	for rows.Next() {
		var name string
		var schema, table *string
		if err := rows.Scan(&name, &schema, &table); err != nil {
			return nil, err
		}
		pub, ok := byName[name]
		if !ok {
			pub = &livePublication{Name: name}
			byName[name] = pub
			order = append(order, name)
		}
		if schema != nil && table != nil && *schema != "" && *table != "" {
			pub.Tables = append(pub.Tables, *schema+"."+*table)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]livePublication, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out, nil
}

func inspectColumns(ctx context.Context, conn *pgx.Conn, schema, name string) ([]liveColumn, error) {
	rel := schema + "." + name
	pkRows, err := conn.Query(ctx, `
SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
WHERE i.indrelid = $1::regclass AND i.indisprimary`, rel)
	if err != nil {
		return nil, err
	}
	pks := map[string]bool{}
	for pkRows.Next() {
		var col string
		if err := pkRows.Scan(&col); err != nil {
			pkRows.Close()
			return nil, err
		}
		pks[col] = true
	}
	if err := pkRows.Err(); err != nil {
		pkRows.Close()
		return nil, err
	}
	pkRows.Close()
	rows, err := conn.Query(ctx, `
SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod), NOT a.attnotnull, COALESCE(pg_get_expr(ad.adbin, ad.adrelid), '')
FROM pg_attribute a
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, rel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []liveColumn
	for rows.Next() {
		var col liveColumn
		var pgType, def string
		if err := rows.Scan(&col.Name, &pgType, &col.Nullable, &def); err != nil {
			return nil, err
		}
		col.Type = mapLiveType(pgType, def)
		col.PrimaryKey = pks[col.Name]
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func mapLiveType(pgType, def string) string {
	base := pgType
	if i := strings.IndexByte(pgType, '('); i >= 0 {
		base = pgType[:i]
	}
	if strings.HasPrefix(def, "nextval(") && base == "integer" {
		return "serial"
	}
	if base == "timestamp with time zone" {
		return "timestamptz"
	}
	return base
}

func applySQL(master instanceLogin, plan applyPlan) error {
	ctx := context.Background()
	owner, cdc := plan.Connection.User, plan.CDC.User
	if err := ensureRole(ctx, master, owner); err != nil {
		return err
	}
	if err := ensureRole(ctx, master, cdc); err != nil {
		return err
	}
	if err := execSQL(ctx, master, "postgres", grantReplicationSQL(cdc)); err != nil {
		return err
	}
	if err := execSQL(ctx, master, "postgres", grantRDSIAMSQL(owner)); err != nil {
		return err
	}
	if err := execSQL(ctx, master, "postgres", grantRDSIAMSQL(cdc)); err != nil {
		return err
	}
	if plan.Database.DDL != "" {
		if err := execSQL(ctx, master, "postgres", plan.Database.DDL); err != nil {
			return err
		}
	}
	if err := grantDatabase(ctx, master, owner, cdc, plan.Database.Name); err != nil {
		return err
	}
	if err := grantSchemas(ctx, master, owner, cdc, plan); err != nil {
		return err
	}
	for _, tbl := range plan.Tables {
		if err := execSQL(ctx, master, tbl.Database, tbl.DDL); err != nil {
			return err
		}
		if err := execSQL(ctx, master, tbl.Database, alterTableOwnerSQL(tbl.Schema, tbl.Name, owner)); err != nil {
			return err
		}
		if err := execSQL(ctx, master, tbl.Database, grantSelectSQL(tbl.Schema, tbl.Name, cdc)); err != nil {
			return err
		}
	}
	if plan.Connector.PublicationDDL != "" {
		if err := execSQL(ctx, master, plan.Database.Name, plan.Connector.PublicationDDL); err != nil {
			return err
		}
	}
	if plan.Connector.PublicationOwner != "" {
		if err := execSQL(ctx, master, plan.Database.Name, plan.Connector.PublicationOwner); err != nil {
			return err
		}
	}
	for _, add := range plan.Connector.PublicationAdds {
		if err := execSQL(ctx, master, plan.Database.Name, add); err != nil {
			return err
		}
	}
	return nil
}

func ensureRole(ctx context.Context, master instanceLogin, name string) error {
	conn, err := pgx.Connect(ctx, postgresURL(master, "postgres"))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = conn.Exec(ctx, "CREATE ROLE "+name+" WITH LOGIN")
	return err
}

func grantDatabase(ctx context.Context, master instanceLogin, owner, cdc, database string) error {
	conn, err := pgx.Connect(ctx, postgresURL(master, "postgres"))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	for _, sql := range []string{
		grantOwnerDatabaseSQL(owner, database),
		grantCDCDatabaseSQL(cdc, database),
		revokeCreateDatabaseSQL(cdc, database),
		revokePublicConnectSQL(database),
		alterDatabaseOwnerSQL(database, owner),
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func grantSchemas(ctx context.Context, master instanceLogin, owner, cdc string, plan applyPlan) error {
	conn, err := pgx.Connect(ctx, postgresURL(master, plan.Database.Name))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	seen := map[string]bool{}
	for _, tbl := range plan.Tables {
		if seen[tbl.Schema] {
			continue
		}
		seen[tbl.Schema] = true
		if tbl.Schema != "public" {
			if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+tbl.Schema); err != nil {
				return err
			}
		}
		for _, sql := range []string{
			grantOwnerSchemaSQL(owner, tbl.Schema),
			grantCDCSchemaSQL(cdc, tbl.Schema),
			revokeCreateSchemaSQL(cdc, tbl.Schema),
			alterDefaultPrivilegesSQL(owner, tbl.Schema, cdc),
		} {
			if _, err := conn.Exec(ctx, sql); err != nil {
				return err
			}
		}
	}
	return nil
}

func grantReplicationSQL(role string) string {
	return "GRANT rds_replication TO " + role
}

func grantOwnerDatabaseSQL(role, database string) string {
	return "GRANT CONNECT, CREATE ON DATABASE " + database + " TO " + role
}

func grantCDCDatabaseSQL(role, database string) string {
	return "GRANT CONNECT ON DATABASE " + database + " TO " + role
}

func revokeCreateDatabaseSQL(role, database string) string {
	return "REVOKE CREATE ON DATABASE " + database + " FROM " + role
}

func revokePublicConnectSQL(database string) string {
	return "REVOKE CONNECT ON DATABASE " + database + " FROM PUBLIC"
}

func grantOwnerSchemaSQL(role, schema string) string {
	return "GRANT USAGE, CREATE ON SCHEMA " + schema + " TO " + role
}

func grantCDCSchemaSQL(role, schema string) string {
	return "GRANT USAGE ON SCHEMA " + schema + " TO " + role
}

func revokeCreateSchemaSQL(role, schema string) string {
	return "REVOKE CREATE ON SCHEMA " + schema + " FROM " + role
}

func grantSelectSQL(schema, table, role string) string {
	return "GRANT SELECT ON TABLE " + schema + "." + table + " TO " + role
}

func alterDefaultPrivilegesSQL(owner, schema, cdc string) string {
	return "ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA " + schema + " GRANT SELECT ON TABLES TO " + cdc
}

func alterTableOwnerSQL(schema, table, role string) string {
	return "ALTER TABLE " + schema + "." + table + " OWNER TO " + role
}

func alterDatabaseOwnerSQL(database, role string) string {
	return "ALTER DATABASE " + database + " OWNER TO " + role
}

func createPublicationSQL(name string, tables []plannedTable) string {
	var b strings.Builder
	b.WriteString("CREATE PUBLICATION ")
	b.WriteString(name)
	b.WriteString(" FOR TABLE ")
	for i, tbl := range tables {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tbl.Schema)
		b.WriteByte('.')
		b.WriteString(tbl.Name)
	}
	return b.String()
}

func alterPublicationAddSQL(name, schema, table string) string {
	return "ALTER PUBLICATION " + name + " ADD TABLE " + schema + "." + table
}

func alterPublicationOwnerSQL(name, role string) string {
	return "ALTER PUBLICATION " + name + " OWNER TO " + role
}

func grantRDSIAMSQL(role string) string {
	return "GRANT rds_iam TO " + role
}

func execSQL(ctx context.Context, login instanceLogin, database, sql string) error {
	conn, err := pgx.Connect(ctx, postgresURL(login, database))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sql)
	return err
}

func postgresURL(login instanceLogin, database string) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(login.User, login.Password),
		Host:   net.JoinHostPort(login.Host, postgresPort),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", "require")
	u.RawQuery = q.Encode()
	return u.String()
}
