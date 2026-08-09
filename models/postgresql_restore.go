package models

import "strings"

func PostgreSQLRestoreVerifyStatement() string {
	return "SELECT 1"
}

func PostgreSQLRestoreDatabaseExistsStatement(database string) string {
	return "SELECT 1 FROM pg_database WHERE datname = " + postgreSQLLiteral(database)
}

func PostgreSQLRestoreCreateDatabaseStatement(database, owner string) string {
	return "CREATE DATABASE " + postgreSQLIdentifier(database) +
		" WITH TEMPLATE template0 OWNER " + postgreSQLIdentifier(owner)
}

func PostgreSQLRestoreDropDatabaseStatement(database string) string {
	return "DROP DATABASE IF EXISTS " + postgreSQLIdentifier(database) + " WITH (FORCE)"
}

func PostgreSQLRestoreRenameDatabaseStatement(from, to string) string {
	return "ALTER DATABASE " + postgreSQLIdentifier(from) + " RENAME TO " + postgreSQLIdentifier(to)
}

func PostgreSQLRestoreAllowConnectionsStatement(database string, allowed bool) string {
	value := "false"
	if allowed {
		value = "true"
	}
	return "ALTER DATABASE " + postgreSQLIdentifier(database) + " ALLOW_CONNECTIONS " + value
}

func PostgreSQLRestoreTerminateConnectionsStatement(database string) string {
	return "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = " +
		postgreSQLLiteral(database) + " AND pid <> pg_backend_pid()"
}

func postgreSQLIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func postgreSQLLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}
