// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Copying an existing install is what makes this feature adoptable by the
// installed base rather than reserved for new ones. One rule decides what
// travels: a table is carried if its content cannot be reproduced without
// human intervention. Everything else is re-reported by the fleet or
// recomputed, and saying so before writing is part of the contract (FR-026).

// Sentinels the command turns into exit codes.
var (
	// ErrTargetNotEmpty refuses to write into a database that already holds
	// something (FR-027). Merging would need conflict rules nothing can settle.
	ErrTargetNotEmpty = errors.New("target database is not empty")
	// ErrSourceBehind refuses a source whose schema is not at the head: the
	// copy would carry a shape the target's schema does not have.
	ErrSourceBehind = errors.New("source database is behind the embedded schema")
	// ErrCountMismatch means a table did not arrive whole. The transaction is
	// rolled back, so the target stays empty.
	ErrCountMismatch = errors.New("row counts differ between source and target")
	// ErrCopyRefused is returned when the operator declines at the prompt.
	ErrCopyRefused = errors.New("copy refused")
)

// carriedTable is one table that travels, with the reason it does. The order
// of this list is the foreign-key insertion order and is not alphabetical:
// parents before children.
type carriedTable struct {
	Name   string
	Reason string
}

// carriedTables is the complete list, in insertion order. Its grouping mirrors
// data-model.md; a table absent from here is left behind on purpose.
var carriedTables = []carriedTable{
	// Fleet identity: the whole motive. Without these, someone walks to every
	// machine.
	{"agents", "agent identities: without them the fleet must be re-enrolled by hand"},
	{"enrollment_tokens", "enrolment tokens, including the ones not yet used"},

	// Reference anchor: ids are deterministic and agents will re-emit them
	// identically, but resource_alert_configs' foreign keys must hold on insert.
	{"containers", "container rows the per-container thresholds point at"},

	// Declared monitors: label-derived ones would be rediscovered, hand-created
	// ones would not. The table is not split in two.
	{"endpoints", "declared endpoint monitors"},
	{"heartbeats", "declared heartbeat monitors"},
	{"cert_monitors", "declared certificate monitors"},

	// Alerting configuration, channel secrets included.
	{"notification_channels", "notification channels and their secrets"},
	{"alert_triggers", "alert routing"},
	{"alert_trigger_channels", "which channel each trigger notifies"},
	{"silence_rules", "silences in force"},
	{"escalation_policies", "escalation policies"},
	{"webhook_subscriptions", "webhook subscriptions"},

	// Per-container thresholds, typed by hand.
	{"resource_alert_configs", "per-container alert thresholds"},

	// Status page: editorial content and layout.
	{"status_components", "status page components"},
	{"status_component_monitors", "what each component aggregates"},
	{"status_page_settings", "status page settings"},
	{"status_page_assets", "status page assets"},
	{"status_page_faq_items", "status page FAQ"},
	{"status_page_footer_links", "status page footer links"},

	// Consent does not rebuild itself: losing this means asking third parties
	// to subscribe again.
	{"status_subscribers", "status page subscribers, whose consent cannot be rebuilt"},

	// Public communication, written by a human and often linked from outside.
	{"incidents", "published incidents"},
	{"incident_updates", "incident updates"},
	{"incident_components", "which components each incident affects"},

	// Planned ahead, sometimes for after the copy.
	{"maintenance_windows", "planned maintenance windows"},
	{"maintenance_components", "which components each window covers"},

	// Operator decisions: each one is a human call, and losing it brings back
	// noise already dealt with.
	{"update_exclusions", "update exclusions"},
	{"version_pins", "version pins"},
	{"risk_acknowledgments", "acknowledged security findings"},
}

// leftBehind groups what the fleet rebuilds on its own, for the announcement.
type leftBehindGroup struct {
	Group  string
	Tables []string
	Why    string
}

var leftBehindGroups = []leftBehindGroup{
	{"Agent reports", []string{"swarm_nodes", "swarm_services", "swarm_tasks",
		"kubernetes_namespaces", "kubernetes_workloads", "kubernetes_pods",
		"kubernetes_nodes", "kubernetes_events"},
		"re-sent whole, the full inventory passes every 30s"},
	{"Check history", []string{"check_results", "cert_check_results", "cert_chain_entries",
		"heartbeat_pings", "heartbeat_executions"}, "starts again, fills itself"},
	{"Resource history", []string{"resource_snapshots", "resource_hourly", "resource_daily"},
		"same, and it is the bulk of the volume"},
	{"State history", []string{"state_transitions"}, "same"},
	{"Alerts and deliveries", []string{"alerts", "notification_deliveries",
		"escalation_runs", "escalation_deliveries"},
		"an alert still active is re-evaluated and crosses its threshold again"},
	{"Intelligence", []string{"image_updates", "image_update_scans", "container_cves",
		"cve_cache", "risk_score_history", "digest_baselines"}, "recomputed on the next scan"},
	{"Ephemeral tokens", []string{"mcp_oauth_codes", "mcp_oauth_tokens"},
		"short-lived, re-issued by clients"},
	{"Engine state", []string{"schema_meta", "instances"},
		"specific to the local file and to each running process"},
}

// visibleConsequences are the two effects an operator would otherwise discover
// afterwards. FR-026 requires saying them before writing, not after.
var visibleConsequences = []string{
	"An alert acknowledged before the copy comes back unacknowledged if it is still active: " +
		"the acknowledgement lives on the alert row, which does not travel. One click to redo, but you should know.",
	"Curves start from zero: resource, uptime and check history. Aggregates rebuild at their usual intervals.",
}

// Plan is what the copy announces before writing anything.
type Plan struct {
	// Carried lists each travelling table with its row count in the source.
	Carried []TableCount
	// LeftBehind lists what stays, grouped, with the reason.
	LeftBehind []leftBehindGroup
	// Consequences are the visible effects of what stays behind.
	Consequences []string
}

// TableCount is one table and how many rows it holds.
type TableCount struct {
	Table  string
	Reason string
	Rows   int64
}

// Total returns how many rows the copy will move.
func (p Plan) Total() int64 {
	var n int64
	for _, c := range p.Carried {
		n += c.Rows
	}
	return n
}

// Report is what the copy returns once it is done: the same tables, with the
// counts read back from the target.
type Report struct {
	Plan     Plan
	Copied   []TableCount
	Verified bool
}

// copyBatchSize is how many rows go in one multi-row INSERT. Large enough to
// keep the round trips down, small enough to stay under the parameter limit
// for a wide table.
const copyBatchSize = 200

// Copy carries an existing SQLite install into an empty PostgreSQL database,
// in a single transaction. It installs the schema itself rather than letting
// the migrator do it first: PostgreSQL DDL is transactional, so a failure
// half-way rolls the schema back with the data and leaves the target actually
// empty, which is what FR-028 asks for and what makes a retry work.
//
// confirm is called with the plan before anything is written; returning false
// aborts with ErrCopyRefused. out receives the human-readable announcement.
func Copy(ctx context.Context, src, dst *sql.DB, out io.Writer, confirm func(Plan) bool) (Report, error) {
	var report Report

	if err := checkSourceAtHead(ctx, src); err != nil {
		return report, err
	}
	if err := checkTargetEmpty(ctx, dst); err != nil {
		return report, err
	}

	plan, err := buildPlan(ctx, src)
	if err != nil {
		return report, err
	}
	report.Plan = plan

	if out != nil {
		writePlan(out, plan)
	}
	if confirm != nil && !confirm(plan) {
		return report, ErrCopyRefused
	}

	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin copy transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := installSchema(ctx, tx); err != nil {
		return report, err
	}

	copied := make([]TableCount, 0, len(carriedTables))
	for _, t := range plan.Carried {
		n, err := copyTable(ctx, src, tx, t.Table)
		if err != nil {
			return report, err
		}
		if n != t.Rows {
			return report, fmt.Errorf("%w: %s has %d rows in the source, %d arrived",
				ErrCountMismatch, t.Table, t.Rows, n)
		}
		copied = append(copied, TableCount{Table: t.Table, Reason: t.Reason, Rows: n})
	}

	// Read the counts back from the target inside the same transaction: what
	// the report claims is what the database holds, not what we sent.
	for i, c := range copied {
		var n int64
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+c.Table).Scan(&n); err != nil {
			return report, fmt.Errorf("count %s in target: %w", c.Table, err)
		}
		if n != c.Rows {
			return report, fmt.Errorf("%w: %s holds %d rows in the target, %d were copied",
				ErrCountMismatch, c.Table, n, c.Rows)
		}
		copied[i].Rows = n
	}

	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("commit copy: %w", err)
	}
	committed = true

	report.Copied = copied
	report.Verified = true
	return report, nil
}

// checkSourceAtHead refuses a source that has not been migrated to the head:
// its shape would not match the schema the target gets.
func checkSourceAtHead(ctx context.Context, src *sql.DB) error {
	head, err := EmbeddedHeadVersion(DialectSQLite)
	if err != nil {
		return err
	}
	var version uint
	err = src.QueryRowContext(ctx, "SELECT version FROM schema_migrations LIMIT 1").Scan(&version)
	if err != nil {
		return fmt.Errorf("%w: cannot read its schema version (start the binary on it once): %w",
			ErrSourceBehind, err)
	}
	if version != head {
		return fmt.Errorf("%w: source is at %d, this binary carries %d (start the binary on the source once)",
			ErrSourceBehind, version, head)
	}
	return nil
}

// checkTargetEmpty refuses a target that holds anything at all, before writing
// (FR-027).
func checkTargetEmpty(ctx context.Context, dst *sql.DB) error {
	var n int
	err := dst.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()`).Scan(&n)
	if err != nil {
		return fmt.Errorf("inspect target database: %w", err)
	}
	if n != 0 {
		return fmt.Errorf("%w: it already holds %d tables", ErrTargetNotEmpty, n)
	}
	return nil
}

// installSchema applies the embedded PostgreSQL migrations inside the copy's
// transaction and seeds schema_migrations at the head, so a later start finds
// nothing pending.
func installSchema(ctx context.Context, tx *sql.Tx) error {
	entries, err := fs.ReadDir(migrationFS, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("read embedded postgres migrations: %w", err)
	}
	type migration struct {
		version uint64
		name    string
	}
	var ups []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		v, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 32)
		if err != nil {
			return fmt.Errorf("malformed migration name %q: %w", name, err)
		}
		ups = append(ups, migration{v, name})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].version < ups[j].version })

	var head uint64
	for _, m := range ups {
		body, err := fs.ReadFile(migrationFS, "migrations/postgres/"+m.name)
		if err != nil {
			return fmt.Errorf("read %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
		head = m.version
	}

	// Same table shape golang-migrate uses, so it takes over from here.
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT NOT NULL PRIMARY KEY,
		dirty BOOLEAN NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)", head); err != nil {
		return fmt.Errorf("seed schema_migrations: %w", err)
	}
	return nil
}

// buildPlan counts what will travel, so the announcement is exact.
func buildPlan(ctx context.Context, src *sql.DB) (Plan, error) {
	plan := Plan{
		Carried:      make([]TableCount, 0, len(carriedTables)),
		LeftBehind:   leftBehindGroups,
		Consequences: visibleConsequences,
	}
	for _, t := range carriedTables {
		var n int64
		if err := src.QueryRowContext(ctx, "SELECT count(*) FROM "+t.Name).Scan(&n); err != nil {
			return plan, fmt.Errorf("count %s in source: %w", t.Name, err)
		}
		plan.Carried = append(plan.Carried, TableCount{Table: t.Name, Reason: t.Reason, Rows: n})
	}
	return plan, nil
}

// copyTable moves one table, in batches, preserving column order by reading
// the target's own column list.
func copyTable(ctx context.Context, src *sql.DB, tx *sql.Tx, table string) (int64, error) {
	columns, err := tableColumns(ctx, tx, table)
	if err != nil {
		return 0, err
	}
	if len(columns) == 0 {
		return 0, fmt.Errorf("table %s has no columns in the target", table)
	}
	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = `"` + c + `"`
	}
	columnList := strings.Join(quoted, ", ")

	rows, err := src.QueryContext(ctx, "SELECT "+columnList+" FROM "+table)
	if err != nil {
		return 0, fmt.Errorf("read %s from source: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		total int64
		batch []any
		count int
	)
	flush := func() error {
		if count == 0 {
			return nil
		}
		if err := insertBatch(ctx, tx, table, columnList, len(columns), count, batch); err != nil {
			return err
		}
		total += int64(count)
		batch = batch[:0]
		count = 0
		return nil
	}

	for rows.Next() {
		values := make([]any, len(columns))
		scan := make([]any, len(columns))
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return 0, fmt.Errorf("scan %s: %w", table, err)
		}
		batch = append(batch, values...)
		count++
		if count >= copyBatchSize {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate %s: %w", table, err)
	}
	if err := flush(); err != nil {
		return 0, err
	}
	return total, nil
}

// insertBatch writes rowCount rows in one statement.
func insertBatch(ctx context.Context, tx *sql.Tx, table, columnList string, cols, rowCount int, args []any) error {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(table)
	b.WriteString(" (")
	b.WriteString(columnList)
	b.WriteString(") VALUES ")

	n := 1
	for r := range rowCount {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for c := range cols {
			if c > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
		}
		b.WriteByte(')')
	}
	// The sentinel agent is installed by the baseline; the source carries its
	// own row for the same id. Updating keeps the source's view.
	if table == "agents" {
		b.WriteString(` ON CONFLICT (id) DO UPDATE SET
			hostname = excluded.hostname, label = excluded.label,
			os_arch = excluded.os_arch, agent_version = excluded.agent_version,
			detected_runtime = excluded.detected_runtime, status = excluded.status,
			last_seen_at = excluded.last_seen_at, created_at = excluded.created_at`)
	}

	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		return fmt.Errorf("insert into %s: %w", table, err)
	}
	return nil
}

// tableColumns reads the target's column list in ordinal order.
func tableColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("read columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan column of %s: %w", table, err)
		}
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

// writePlan renders the announcement: what travels with its counts, what does
// not, and the consequences — before anything is written (FR-026).
func writePlan(out io.Writer, plan Plan) {
	_, _ = fmt.Fprintf(out, "Carrying %d rows across %d tables:\n\n", plan.Total(), len(plan.Carried))
	for _, c := range plan.Carried {
		_, _ = fmt.Fprintf(out, "  %-28s %8d  %s\n", c.Table, c.Rows, c.Reason)
	}

	_, _ = fmt.Fprintf(out, "\nLeaving behind, because the fleet rebuilds it:\n\n")
	for _, g := range plan.LeftBehind {
		_, _ = fmt.Fprintf(out, "  %-22s %s\n", g.Group, g.Why)
		_, _ = fmt.Fprintf(out, "  %-22s   %s\n", "", strings.Join(g.Tables, ", "))
	}

	_, _ = fmt.Fprintf(out, "\nWhat you will notice:\n\n")
	for _, c := range plan.Consequences {
		_, _ = fmt.Fprintf(out, "  - %s\n", c)
	}
	_, _ = fmt.Fprintln(out)
}
