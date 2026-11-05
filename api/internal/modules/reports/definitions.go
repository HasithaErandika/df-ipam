// Package reports builds registers and evidence reports in the background. Every
// query reads a fixed statement with parameters: a request chooses a report and
// filters from allowlists and never supplies SQL or column names.
package reports

import "dfipam/api/internal/auth"

// Definition is one report. Query parameters are always, in order:
// $1 routing domain, $2 environment, $3 from, $4 to, $5 business-service scope.
// A null means no filter. $5 null means everything, an empty array means nothing.
type Definition struct {
	Key         string   `json:"key"`
	Group       string   `json:"group"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Columns     []string `json:"columns"`
	Filters     []string `json:"filters"`
	// Scoped reports can be limited to a person's business services. Others need
	// full export rights.
	Scoped     bool   `json:"scoped"`
	Permission string `json:"-"`
	sql        string
}

const scopeIP = `($5::uuid[] IS NULL OR i.business_service_id = ANY($5::uuid[]))`
const scopeSubnet = `($5::uuid[] IS NULL OR s.business_service_id = ANY($5::uuid[]))`
const domainIP = `($1::uuid IS NULL OR i.routing_domain_id = $1::uuid)`
const domainSubnet = `($1::uuid IS NULL OR s.routing_domain_id = $1::uuid)`
const envSubnet = `($2::text IS NULL OR s.environment = $2::text)`

// unused keeps every statement's parameter types known, whichever it reads.
const unused = ` AND ($1::uuid IS NULL OR true) AND ($2::text IS NULL OR true) AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) AND ($5::uuid[] IS NULL OR true)`

// Definitions lists every report, in display order.
var Definitions = []Definition{
	{Key: "allocation_register", Group: "Registers", Title: "IP allocation register", Description: "Every tracked address with its allocation and observation state.",
		Columns: []string{"Routing domain", "Subnet", "Address", "Hostname", "MAC", "Allocation state", "Observation state", "Owner", "Business service", "Last seen"},
		Filters: []string{"routingDomainId", "environment"}, Scoped: true, Permission: auth.ReportsExport,
		sql: `SELECT rd.name, s.cidr::text, host(i.address), i.hostname, COALESCE(i.mac::text, ''), i.allocation_state, i.observation_state, COALESCE(u.email, ''), COALESCE(b.name, ''), COALESCE(i.last_seen::text, '')
		FROM ip_addresses i JOIN subnets s ON s.id = i.subnet_id JOIN routing_domains rd ON rd.id = i.routing_domain_id
		LEFT JOIN users u ON u.id = i.owner_id LEFT JOIN business_services b ON b.id = i.business_service_id
		WHERE ` + domainIP + ` AND ` + envSubnet + ` AND ` + scopeIP + ` AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) ORDER BY rd.name, i.address`},
	{Key: "subnet_register", Group: "Registers", Title: "Subnet register", Description: "Every active subnet with its mapping and owners.",
		Columns: []string{"Routing domain", "CIDR", "Name", "Environment", "Site", "VLAN", "VDOM", "Business service", "Technical owner", "Business owner"},
		Filters: []string{"routingDomainId", "environment"}, Scoped: true, Permission: auth.ReportsExport,
		sql: `SELECT rd.name, s.cidr::text, s.name, s.environment, COALESCE(si.name, ''), COALESCE(v.vlan_id::text, ''), COALESCE(vd.name, ''), COALESCE(b.name, ''), COALESCE(tu.email, ''), COALESCE(ou.email, '')
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id LEFT JOIN sites si ON si.id = s.site_id LEFT JOIN vlans v ON v.id = s.vlan_id
		LEFT JOIN vdoms vd ON vd.id = s.vdom_id LEFT JOIN business_services b ON b.id = s.business_service_id
		LEFT JOIN users tu ON tu.id = s.technical_owner_id LEFT JOIN users ou ON ou.id = s.business_owner_id
		WHERE s.archived_at IS NULL AND ` + domainSubnet + ` AND ` + envSubnet + ` AND ` + scopeSubnet + ` AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) ORDER BY rd.name, s.cidr`},
	{Key: "device_inventory", Group: "Registers", Title: "Device inventory", Description: "Devices from every source, with owner and status.",
		Columns: []string{"Name", "Type", "Vendor", "Model", "Serial", "Management IP", "OS", "Status", "Source", "Owner", "Last discovered"},
		Filters: []string{"environment"}, Scoped: true, Permission: auth.ReportsExport,
		sql: `SELECT d.name, d.device_type, d.vendor, d.model, COALESCE(d.serial_number, ''), COALESCE(host(d.management_ip), ''), d.os_version, d.status, d.source_system, COALESCE(u.email, ''), COALESCE(d.last_discovered::text, '')
		FROM devices d LEFT JOIN users u ON u.id = d.owner_id
		WHERE ($2::text IS NULL OR d.environment = $2::text) AND ($5::uuid[] IS NULL OR d.business_service_id = ANY($5::uuid[]))
		  AND ($1::uuid IS NULL OR true) AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) ORDER BY d.name`},
	{Key: "unused_candidates", Group: "Capacity", Title: "Unused address candidates", Description: "Recorded addresses marked available and not seen active. A candidate is not proof an address is unused.",
		Columns: []string{"Routing domain", "Subnet", "Address", "Observation state", "Last seen"},
		Filters: []string{"routingDomainId", "environment"}, Permission: auth.ReportsExport,
		sql: `SELECT rd.name, s.cidr::text, host(i.address), i.observation_state, COALESCE(i.last_seen::text, '')
		FROM ip_addresses i JOIN subnets s ON s.id = i.subnet_id JOIN routing_domains rd ON rd.id = i.routing_domain_id
		WHERE i.allocation_state = 'available_candidate' AND i.observation_state <> 'observed_active' AND ` + domainIP + ` AND ` + envSubnet + `
		  AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) AND ($5::uuid[] IS NULL OR true) ORDER BY rd.name, i.address`},
	{Key: "utilisation", Group: "Capacity", Title: "Subnet utilisation and trends", Description: "Daily utilisation snapshots for the chosen period.",
		Columns: []string{"Date", "Routing domain", "CIDR", "Name", "Allocated", "Reserved", "Observed active", "Utilisation %"},
		Filters: []string{"routingDomainId", "environment", "from", "to"}, Permission: auth.ReportsExport,
		sql: `SELECT to_char(n.captured_at, 'YYYY-MM-DD'), rd.name, s.cidr::text, s.name, n.allocated::text, n.reserved::text, n.observed_active::text, n.utilisation_pct::text
		FROM subnet_utilisation_snapshots n JOIN subnets s ON s.id = n.subnet_id JOIN routing_domains rd ON rd.id = s.routing_domain_id
		WHERE ` + domainSubnet + ` AND ` + envSubnet + ` AND ($3::timestamptz IS NULL OR n.captured_at >= $3::timestamptz) AND ($4::timestamptz IS NULL OR n.captured_at < $4::timestamptz)
		  AND ($5::uuid[] IS NULL OR true) ORDER BY n.captured_at DESC, rd.name, s.cidr`},
	{Key: "discovery_evidence", Group: "Discovery", Title: "Discovery evidence", Description: "Raw observations for the chosen period, kept apart from inventory.",
		Columns: []string{"Observed", "Source", "Routing domain", "Address", "MAC", "Hostname"},
		Filters: []string{"routingDomainId", "from", "to"}, Permission: auth.ReportsExport,
		sql: `SELECT o.observed_at::text, os.name, COALESCE(rd.name, 'Not mapped'), COALESCE(host(o.address), ''), COALESCE(o.mac::text, ''), COALESCE(o.hostname, '')
		FROM discovery_observations o JOIN observation_sources os ON os.id = o.source_id LEFT JOIN routing_domains rd ON rd.id = o.routing_domain_id
		WHERE ($1::uuid IS NULL OR o.routing_domain_id = $1::uuid) AND ($3::timestamptz IS NULL OR o.observed_at >= $3::timestamptz) AND ($4::timestamptz IS NULL OR o.observed_at < $4::timestamptz)
		  AND ($2::text IS NULL OR true) AND ($5::uuid[] IS NULL OR true) ORDER BY o.observed_at DESC`},
	{Key: "discovery_coverage", Group: "Discovery", Title: "Discovery coverage", Description: "Which subnets have recent evidence, and which have none.",
		Columns: []string{"Routing domain", "CIDR", "Name", "Discovery enabled", "Records", "Seen in 7 days", "Last evidence"},
		Filters: []string{"routingDomainId", "environment"}, Permission: auth.ReportsExport,
		sql: `SELECT rd.name, s.cidr::text, s.name, s.discovery_enabled::text, count(i.id)::text, (count(i.id) FILTER (WHERE i.last_seen > now() - interval '7 days'))::text, COALESCE(max(i.last_seen)::text, 'Never')
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id LEFT JOIN ip_addresses i ON i.subnet_id = s.id
		WHERE s.archived_at IS NULL AND ` + domainSubnet + ` AND ` + envSubnet + ` AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) AND ($5::uuid[] IS NULL OR true)
		GROUP BY rd.name, s.id ORDER BY rd.name, s.cidr`},
	{Key: "unmanaged_devices", Group: "Discovery", Title: "Unmanaged devices", Description: "Devices reported by a source that nobody owns yet.",
		Columns: []string{"Name", "Type", "Management IP", "Source", "First discovered", "Last discovered"},
		Filters: []string{}, Permission: auth.ReportsExport,
		sql: `SELECT d.name, d.device_type, COALESCE(host(d.management_ip), ''), d.source_system, COALESCE(d.first_discovered::text, ''), COALESCE(d.last_discovered::text, '')
		FROM devices d WHERE d.source_system <> 'manual' AND d.owner_id IS NULL` + unused + ` ORDER BY d.first_discovered DESC NULLS LAST`},
	{Key: "conflict_investigation", Group: "Risk", Title: "Conflict investigation", Description: "Suspected conflicts and duplicate MACs. These need investigation and are not proof.",
		Columns: []string{"Finding", "Routing domain", "Address", "Sources", "Confidence", "Status", "Explanation", "Suggested action"},
		Filters: []string{"routingDomainId"}, Permission: auth.ReportsExport,
		sql: `SELECT r.classification, COALESCE(rd.name, ''), COALESCE(host(r.address), ''), array_to_string(r.sources, ', '), COALESCE(r.confidence::text, ''), r.review_status, r.explanation, r.suggested_action
		FROM reconciliation_results r LEFT JOIN routing_domains rd ON rd.id = r.routing_domain_id
		WHERE r.classification IN ('ip_conflict_suspected', 'duplicate_mac') AND r.review_status IN ('open', 'investigating')
		  AND ($1::uuid IS NULL OR r.routing_domain_id = $1::uuid) AND ($2::text IS NULL OR true) AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) AND ($5::uuid[] IS NULL OR true)
		ORDER BY r.confidence DESC NULLS LAST`},
	{Key: "ownership_gaps", Group: "Risk", Title: "Ownership gaps", Description: "Subnets without a technical or business owner.",
		Columns: []string{"Routing domain", "CIDR", "Name", "Environment", "Technical owner", "Business owner", "Business service"},
		Filters: []string{"routingDomainId", "environment"}, Scoped: true, Permission: auth.ReportsExport,
		sql: `SELECT rd.name, s.cidr::text, s.name, s.environment, COALESCE(tu.email, 'Missing'), COALESCE(ou.email, 'Missing'), COALESCE(b.name, '')
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id LEFT JOIN users tu ON tu.id = s.technical_owner_id LEFT JOIN users ou ON ou.id = s.business_owner_id
		LEFT JOIN business_services b ON b.id = s.business_service_id
		WHERE s.archived_at IS NULL AND (s.technical_owner_id IS NULL OR s.business_owner_id IS NULL) AND ` + domainSubnet + ` AND ` + envSubnet + ` AND ` + scopeSubnet + `
		  AND ($3::timestamptz IS NULL OR true) AND ($4::timestamptz IS NULL OR true) ORDER BY rd.name, s.cidr`},
	{Key: "change_history", Group: "Governance", Title: "Change history", Description: "Approved and rejected inventory changes for the chosen period.",
		Columns: []string{"Requested", "Object", "Status", "Reason", "Requested by", "Decided by", "Decided"},
		Filters: []string{"from", "to"}, Permission: auth.AuditView,
		sql: `SELECT c.created_at::text, c.object_type, c.status, c.reason, COALESCE(ru.email, ''), COALESCE(du.email, ''), COALESCE(c.decided_at::text, '')
		FROM change_requests c LEFT JOIN users ru ON ru.id = c.requested_by LEFT JOIN users du ON du.id = c.decided_by
		WHERE ($3::timestamptz IS NULL OR c.created_at >= $3::timestamptz) AND ($4::timestamptz IS NULL OR c.created_at < $4::timestamptz)
		  AND ($1::uuid IS NULL OR true) AND ($2::text IS NULL OR true) AND ($5::uuid[] IS NULL OR true) ORDER BY c.created_at DESC`},
	{Key: "audit_trail", Group: "Governance", Title: "Audit trail", Description: "Who did what and when, for the chosen period.",
		Columns: []string{"Time", "Actor", "Action", "Object type", "Object ID", "Source IP", "Result", "Correlation ID"},
		Filters: []string{"from", "to"}, Permission: auth.AuditView,
		sql: `SELECT a.occurred_at::text, COALESCE(u.email, a.actor_type), a.action, a.object_type, COALESCE(a.object_id, ''), COALESCE(host(a.source_ip), ''), a.result, COALESCE(a.correlation_id, '')
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($3::timestamptz IS NULL OR a.occurred_at >= $3::timestamptz) AND ($4::timestamptz IS NULL OR a.occurred_at < $4::timestamptz)
		  AND ($1::uuid IS NULL OR true) AND ($2::text IS NULL OR true) AND ($5::uuid[] IS NULL OR true) ORDER BY a.occurred_at DESC, a.id DESC`},
	{Key: "integration_report", Group: "Governance", Title: "Integration report", Description: "Connection health and sync history. Credentials are never included.",
		Columns: []string{"Connection", "Kind", "Health", "Sync status", "Started", "Finished", "Error"},
		Filters: []string{"from", "to"}, Permission: auth.ReportsExport,
		sql: `SELECT c.name, c.kind, c.health, j.status, COALESCE(j.started_at::text, ''), COALESCE(j.finished_at::text, ''), j.error
		FROM integration_sync_jobs j JOIN integration_connections c ON c.id = j.connection_id
		WHERE ($3::timestamptz IS NULL OR j.created_at >= $3::timestamptz) AND ($4::timestamptz IS NULL OR j.created_at < $4::timestamptz)
		  AND ($1::uuid IS NULL OR true) AND ($2::text IS NULL OR true) AND ($5::uuid[] IS NULL OR true) ORDER BY j.created_at DESC`},
}

func definition(key string) (Definition, bool) {
	for _, d := range Definitions {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}
