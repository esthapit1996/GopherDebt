package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/lib/pq"
)

// appTables are the app's tables in the public schema
var appTables = []string{
	"users", "groups", "group_members", "expenses", "expense_splits", "settlements",
	"expense_payments", "activity_log", "suggestions", "suggestion_votes", "suggestion_comments",
	"email_whitelist", "email_blacklist", "stash_expenses", "wish_items", "wish_lists", "wish_list_members",
	"ai_usage", "gocrumbs_profiles", "gocrumbs_expenses", "gocrumbs_extra_pay",
	"gocrumbs_pages", "gocrumbs_page_expenses", "gocrumbs_page_extra_pay", "migration_flags",
}

// Supabase's auto-generated REST API (PostgREST) runs as these roles
var publicAPIRoles = []string{"anon", "authenticated"}

// lockDownPublicAPI closes Supabase's REST API over the app's tables. RLS stays enabled with no
// policies, so anon/authenticated see no rows, and their default table/sequence grants are revoked.
// The app keeps full access because its role owns the tables (owners bypass non-forced RLS) or has
// BYPASSRLS. If that can't be confirmed, nothing is changed and a warning is logged, so a
// misconfigured role can never lock the app out. Only existing policies/grants are touched, so
// after the first run this does no DDL.
func lockDownPublicAPI(d *sql.DB) error {
	var safe bool
	err := d.QueryRow(`
		SELECT r.rolsuper OR r.rolbypassrls OR NOT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname = ANY($1)
			  AND (c.relowner <> r.oid OR c.relforcerowsecurity))
		FROM pg_roles r WHERE r.rolname = current_user`,
		pq.Array(appTables),
	).Scan(&safe)
	if err != nil {
		return fmt.Errorf("check app role: %w", err)
	}
	if !safe {
		log.Printf("WARNING: public API lockdown skipped: app role does not own every app table and has no BYPASSRLS, so the permissive RLS policies were left in place")
		return nil
	}

	// Always-true policies (USING (true) applies to every role, including anon)
	rows, err := d.Query(
		`SELECT tablename, policyname FROM pg_policies
		 WHERE schemaname = 'public' AND tablename = ANY($1)
		   AND (policyname = tablename || '_app_access' OR policyname = 'stash_expenses_self_access')`,
		pq.Array(appTables),
	)
	if err != nil {
		return fmt.Errorf("list policies: %w", err)
	}
	var drops []string
	for rows.Next() {
		var table, policy string
		if err := rows.Scan(&table, &policy); err != nil {
			rows.Close()
			return err
		}
		drops = append(drops, fmt.Sprintf(`DROP POLICY IF EXISTS %s ON %s`, pq.QuoteIdentifier(policy), pq.QuoteIdentifier(table)))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Table grants Supabase gives anon/authenticated by default
	rows, err = d.Query(
		`SELECT DISTINCT grantee, table_name FROM information_schema.role_table_grants
		 WHERE table_schema = 'public' AND grantee = ANY($1) AND table_name = ANY($2)`,
		pq.Array(publicAPIRoles), pq.Array(appTables),
	)
	if err != nil {
		return fmt.Errorf("list grants: %w", err)
	}
	var revokes []string
	for rows.Next() {
		var role, table string
		if err := rows.Scan(&role, &table); err != nil {
			rows.Close()
			return err
		}
		revokes = append(revokes, fmt.Sprintf(`REVOKE ALL ON TABLE %s FROM %s`, pq.QuoteIdentifier(table), pq.QuoteIdentifier(role)))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Sequence grants (nextval/setval on the app's ID sequences)
	for _, role := range publicAPIRoles {
		var has bool
		err := d.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1) AND EXISTS(
				SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public'
				  -- CASE so the privilege check only ever sees sequences (WHERE order isn't guaranteed)
				  AND CASE WHEN c.relkind = 'S' THEN has_sequence_privilege($1, c.oid, 'USAGE,SELECT,UPDATE') ELSE false END)`,
			role,
		).Scan(&has)
		if err != nil {
			return fmt.Errorf("check sequence grants for %s: %w", role, err)
		}
		if has {
			revokes = append(revokes, fmt.Sprintf(`REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM %s`, pq.QuoteIdentifier(role)))
		}
	}

	for _, stmt := range append(drops, revokes...) {
		if _, err := d.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	if len(drops)+len(revokes) > 0 {
		log.Printf("Public API lockdown: dropped %d permissive policies, revoked %d grants", len(drops), len(revokes))
	}
	return nil
}

// ensureUniqueEmailIgnoringCase stops "Alice@x.com" and "alice@x.com" from both registering.
// If such duplicates already exist the index is skipped with a warning instead of failing boot;
// registration still rejects case variants because the lookup ignores case.
func ensureUniqueEmailIgnoringCase(d *sql.DB) error {
	var exists bool
	if err := d.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'idx_users_email_lower')`,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	var dupes int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM (SELECT 1 FROM users GROUP BY LOWER(email) HAVING COUNT(*) > 1) x`,
	).Scan(&dupes); err != nil {
		return err
	}
	if dupes > 0 {
		log.Printf("WARNING: %d email(s) are registered more than once with different letter case; case-insensitive unique index not created until they are merged", dupes)
		return nil
	}

	_, err := d.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email))`)
	return err
}
