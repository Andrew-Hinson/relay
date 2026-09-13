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
	tables, err := inspectDatabase(ctx, login, database)
	if err != nil {
		return liveSnapshot{}, err
	}
	live.Tables = tables
	return live, nil
}

func inspectDatabase(ctx context.Context, login instanceLogin, database string) ([]liveTable, error) {
	conn, err := pgx.Connect(ctx, postgresURL(login, database))
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `
SELECT n.nspname, c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')`)
	if err != nil {
		return nil, err
	}
	var found []liveTable
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, liveTable{Database: database, Schema: schema, Name: name})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range found {
		cols, err := inspectColumns(ctx, conn, found[i].Schema, found[i].Name)
		if err != nil {
			return nil, err
		}
		found[i].Columns = cols
	}
	return found, nil
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

func applySQL(login instanceLogin, plan applyPlan) error {
	ctx := context.Background()
	if plan.Database.DDL != "" {
		if err := execSQL(ctx, login, "postgres", plan.Database.DDL); err != nil {
			return err
		}
	}
	for _, tbl := range plan.Tables {
		if err := execSQL(ctx, login, tbl.Database, tbl.DDL); err != nil {
			return err
		}
	}
	return nil
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
	q.Set("sslmode", "prefer")
	u.RawQuery = q.Encode()
	return u.String()
}
