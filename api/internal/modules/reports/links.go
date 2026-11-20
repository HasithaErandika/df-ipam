package reports

// linkType is a link's type name from the links.types setting, or its key.
const linkType = `COALESCE((SELECT t->>'label' FROM settings st, jsonb_array_elements(st.value) t
	WHERE st.key = 'links.types' AND t->>'key' = l.link_type), l.link_type)`

const linkFrom = ` FROM network_links l LEFT JOIN network_link_providers p ON p.id = l.provider_id LEFT JOIN users u ON u.id = l.officer_id`

const money = `COALESCE(to_char(%s, 'FM999,999,999,990.00'), '')`

// linkReports cover Network Links (docs/product/network-links.md). Archived
// links are left out. Cost and contract reports need commercial access too.
var linkReports = []Report{
	{Key: "network-links", Title: "Network links", Description: "Every network link that isn't archived: type, provider, locations, role, bandwidth, addresses and responsible officer.",
		Needs:   []string{"links:view"},
		Columns: []string{"Link ID", "Name", "Type", "Provider", "Circuit ID", "Source location", "Destination", "Role", "Criticality", "Status", "Download Mbps", "Upload Mbps", "WAN IP", "Public IPs", "Responsible officer", "Last changed"},
		sql: `SELECT l.code, l.name, ` + linkType + `, COALESCE(p.name, ''), l.circuit_id, l.source_site, l.destination, initcap(l.link_role), initcap(l.criticality), initcap(l.status),
			COALESCE(trim_scale(l.down_mbps)::text, ''), COALESCE(trim_scale(l.up_mbps)::text, ''), COALESCE(host(l.wan_ip), ''), COALESCE(l.public_ips::text, ''),
			COALESCE(u.email, ''), to_char(l.updated_at AT TIME ZONE 'Asia/Colombo', 'YYYY-MM-DD HH24:MI')` + linkFrom + `
			WHERE l.archived_at IS NULL AND $1::date IS NOT NULL AND $2::date IS NOT NULL AND $3::text IS NOT NULL ORDER BY l.code`},
	{Key: "network-link-costs", Title: "Network link costs", Description: "Monthly and annual cost of each link that isn't archived or terminated, in its own currency, with billing and contract details.",
		Needs:   []string{"links:view", "links:commercial_view"},
		Columns: []string{"Link ID", "Name", "Provider", "Status", "Monthly rental", "Currency", "Annual recurring cost", "Installation cost", "No separate cost", "Billing account", "Cost centre", "Contract reference", "Contract expiry"},
		sql: `SELECT l.code, l.name, COALESCE(p.name, ''), initcap(l.status), ` + sprintf(money, "l.monthly_rental") + `, l.currency, ` + sprintf(money, "l.monthly_rental * 12") + `,
			` + sprintf(money, "l.install_cost") + `, CASE WHEN l.no_separate_cost THEN 'Yes' ELSE 'No' END, l.billing_account, l.cost_centre, l.contract_ref,
			COALESCE(l.contract_end::text, '')` + linkFrom + `
			WHERE l.archived_at IS NULL AND l.status <> 'terminated' AND $1::date IS NOT NULL AND $2::date IS NOT NULL AND $3::text IS NOT NULL
			ORDER BY l.currency, l.monthly_rental DESC NULLS LAST, l.code`},
	{Key: "network-link-renewals", Title: "Network link contract renewals", Description: "Link contracts ending in the date range, with notice periods and officers. Terminated links are left out.", Dated: true, Ahead: true,
		Needs:   []string{"links:view", "links:commercial_view"},
		Columns: []string{"Link ID", "Name", "Provider", "Contract expiry", "Days left", "Notice period (days)", "Monthly rental", "Currency", "Responsible officer", "Contract reference"},
		sql: `SELECT l.code, l.name, COALESCE(p.name, ''), l.contract_end::text, (l.contract_end - (now() AT TIME ZONE 'Asia/Colombo')::date)::text,
			COALESCE(l.renewal_notice_days::text, ''), ` + sprintf(money, "l.monthly_rental") + `, l.currency, COALESCE(u.email, ''), l.contract_ref` + linkFrom + `
			WHERE l.archived_at IS NULL AND l.status <> 'terminated' AND l.contract_end >= $1::date AND l.contract_end < $2::date + 1 AND $3::text IS NOT NULL
			ORDER BY l.contract_end, l.code`},
}
