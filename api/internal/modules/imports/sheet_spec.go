package imports

// cellKind says how a cell is read and checked.
type cellKind int

const (
	textCell cellKind = iota
	intCell
	boolCell
	cidrCell
	ipCell
	macCell
)

// column is one template column. Names here are constants, never input.
type column struct {
	Name, Field string
	Kind        cellKind
	Required    bool
	Enum        []string
	Ref         string // the sheet, or "people", this column points at
	Within      string // the column that scopes the reference, such as a routing domain
	Example     string
}

// sheet is one type of record. Sheets are listed parents first, so a row can refer
// to a row on an earlier sheet of the same file.
type sheet struct {
	Key, Title, Object string
	Entity             string // catalog path; empty when a module has its own insert
	KeyCol, AltCol     string // what identifies a row, and a second name for it
	Parent             string // the reference column the key is unique within
	Columns            []column
}

func (s *sheet) column(name string) *column {
	for i := range s.Columns {
		if s.Columns[i].Name == name {
			return &s.Columns[i]
		}
	}
	return nil
}

func sheetByKey(key string) *sheet {
	for i := range sheets {
		if sheets[i].Key == key {
			return &sheets[i]
		}
	}
	return nil
}

func sheetByTitle(title string) *sheet {
	for i := range sheets {
		if sheets[i].Title == title {
			return &sheets[i]
		}
	}
	return nil
}

func need(name, field, example string) column {
	return column{Name: name, Field: field, Required: true, Example: example}
}

func opt(name, field, example string) column {
	return column{Name: name, Field: field, Example: example}
}

func ref(name, field, target, example string) column {
	return column{Name: name, Field: field, Ref: target, Example: example}
}

func needRef(name, field, target, example string) column {
	return column{Name: name, Field: field, Ref: target, Required: true, Example: example}
}

func oneOf(name, field, example string, values ...string) column {
	return column{Name: name, Field: field, Enum: values, Example: example}
}

func kind(c column, k cellKind) column {
	c.Kind = k
	return c
}

func within(c column, parent string) column {
	c.Within = parent
	return c
}

func required(c column) column {
	c.Required = true
	return c
}

var (
	environmentValues = []string{"production", "disaster_recovery", "uat", "development", "testing"}
	sensitivityValues = []string{"standard", "sensitive", "restricted"}
	deviceTypeValues  = []string{"firewall", "router", "switch", "access_point", "wireless_controller", "load_balancer", "network_appliance", "physical_server", "virtual_machine",
		"cloud_interface", "storage", "laptop", "desktop", "mobile_phone", "tablet", "ip_phone", "printer", "camera", "iot", "other"}
	osFamilyValues = []string{"unknown", "windows", "windows_server", "linux", "macos", "ios", "android", "network_os", "hypervisor", "other"}
	stateValues    = []string{"allocated", "reserved", "available_candidate", "quarantined", "retired"}
)

var sheets = []sheet{
	{Key: "organisations", Title: "Organisations", Object: "organisation", KeyCol: "code", AltCol: "name", Columns: []column{
		need("name", "name", "Dialog Finance"), need("code", "code", "DF")}},
	{Key: "routing_domains", Title: "Routing domains", Object: "routing_domain", KeyCol: "name", Parent: "organisation", Columns: []column{
		needRef("organisation", "organisationId", "organisations", "DF"), need("name", "name", "Corporate VRF"),
		required(oneOf("kind", "kind", "vrf", "default", "vrf", "vpc", "vnet", "isolated")),
		opt("route_distinguisher", "routeDistinguisher", "65000:100"), kind(opt("is_isolated", "isIsolated", "no"), boolCell)}},
	{Key: "business_units", Title: "Business units", Object: "business_unit", Entity: "/business-units", KeyCol: "code", AltCol: "name", Parent: "organisation", Columns: []column{
		needRef("organisation", "organisationId", "organisations", "DF"), need("name", "name", "Treasury"), need("code", "code", "TRS")}},
	{Key: "sites", Title: "Sites", Object: "site", Entity: "/sites", KeyCol: "code", AltCol: "name", Parent: "organisation", Columns: []column{
		needRef("organisation", "organisationId", "organisations", "DF"), need("name", "name", "Colombo Head Office"), need("code", "code", "CMB"),
		oneOf("site_type", "siteType", "site", "site", "data_centre", "cloud_region", "branch"), opt("address", "address", "Colombo 03")}},
	{Key: "network_zones", Title: "Network zones", Object: "network_zone", Entity: "/network-zones", KeyCol: "name", Parent: "site", Columns: []column{
		ref("site", "siteId", "sites", "CMB"), need("name", "name", "DMZ"), opt("security_zone", "securityZone", "dmz")}},
	{Key: "business_services", Title: "Business services", Object: "business_service", Entity: "/business-services", KeyCol: "name", Columns: []column{
		need("name", "name", "Core Banking"), oneOf("criticality", "criticality", "critical", "low", "standard", "high", "critical"),
		kind(opt("is_sensitive", "isSensitive", "yes"), boolCell)}},
	{Key: "applications", Title: "Applications", Object: "application", Entity: "/applications", KeyCol: "name", Columns: []column{
		need("name", "name", "Payments API"), ref("business_service", "businessServiceId", "business_services", "Core Banking")}},
	{Key: "teams", Title: "Teams", Object: "team", Entity: "/teams", KeyCol: "name", Columns: []column{
		need("name", "name", "Network Operations"), opt("email", "email", "netops@example.com"), opt("description", "description", "Runs the core network")}},
	{Key: "devices", Title: "Devices", Object: "device", Entity: "/devices", KeyCol: "name", Columns: []column{
		need("name", "name", "cmb-fw-01"), opt("hostname", "hostname", "cmb-fw-01.example.com"), kind(opt("management_ip", "managementIp", "10.0.0.1"), ipCell),
		opt("vendor", "vendor", "Fortinet"), opt("model", "model", "FortiGate 200F"), opt("serial_number", "serialNumber", "FG200F0000000"),
		oneOf("device_type", "deviceType", "firewall", deviceTypeValues...), opt("os_version", "osVersion", "7.4.3"), oneOf("os_family", "osFamily", "network_os", osFamilyValues...),
		ref("site", "siteId", "sites", "CMB"), oneOf("environment", "environment", "production", environmentValues...), ref("owner_email", "ownerId", "people", "owner@example.com"),
		ref("business_service", "businessServiceId", "business_services", "Core Banking"), oneOf("status", "status", "active", "active", "inactive", "retired", "unknown"),
		oneOf("sensitivity", "sensitivity", "standard", sensitivityValues...), opt("notes", "notes", "Primary firewall")}},
	{Key: "vdoms", Title: "VDOMs", Object: "vdom", Entity: "/vdoms", KeyCol: "name", Parent: "device", Columns: []column{
		needRef("device", "deviceId", "devices", "cmb-fw-01"), need("name", "name", "root"), ref("routing_domain", "routingDomainId", "routing_domains", "Corporate VRF")}},
	{Key: "vlans", Title: "VLANs", Object: "vlan", Entity: "/vlans", KeyCol: "vlan_id", Parent: "site", Columns: []column{
		kind(need("vlan_id", "vlanId", "100"), intCell), opt("name", "name", "Users"), ref("site", "siteId", "sites", "CMB"), ref("network_zone", "networkZoneId", "network_zones", "DMZ")}},
	{Key: "interfaces", Title: "Interfaces", Object: "device_interface", Entity: "/interfaces", KeyCol: "name", Parent: "device", Columns: []column{
		needRef("device", "deviceId", "devices", "cmb-fw-01"), need("name", "name", "port1"), kind(opt("mac", "mac", "00:09:0f:aa:bb:cc"), macCell),
		opt("interface_type", "interfaceType", "physical"), opt("admin_status", "adminStatus", "up"), opt("oper_status", "operStatus", "up"),
		ref("vlan", "vlanId", "vlans", "100"), ref("routing_domain", "routingDomainId", "routing_domains", "Corporate VRF"), opt("description", "description", "Uplink")}},
	{Key: "network_blocks", Title: "Network blocks", Object: "network_block", Entity: "/network-blocks", KeyCol: "cidr", Parent: "routing_domain", Columns: []column{
		needRef("routing_domain", "routingDomainId", "routing_domains", "Corporate VRF"), kind(need("cidr", "cidr", "10.0.0.0/16"), cidrCell),
		need("name", "name", "Head office block"), opt("purpose", "purpose", "Corporate users")}},
	{Key: "subnets", Title: "Subnets", Object: "subnet", KeyCol: "cidr", Parent: "routing_domain", Columns: []column{
		needRef("routing_domain", "routingDomainId", "routing_domains", "Corporate VRF"), kind(need("cidr", "cidr", "10.0.1.0/24"), cidrCell), need("name", "name", "Users floor 1"),
		required(oneOf("environment", "environment", "production", environmentValues...)), kind(opt("gateway", "gateway", "10.0.1.1"), ipCell),
		ref("site", "siteId", "sites", "CMB"), ref("network_zone", "networkZoneId", "network_zones", "DMZ"), ref("vlan", "vlanId", "vlans", "100"), ref("vdom", "vdomId", "vdoms", "root"),
		within(ref("network_block", "networkBlockId", "network_blocks", "10.0.0.0/16"), "routing_domain"), ref("business_unit", "businessUnitId", "business_units", "TRS"),
		ref("business_service", "businessServiceId", "business_services", "Core Banking"), ref("technical_owner_email", "technicalOwnerId", "people", "owner@example.com"),
		ref("business_owner_email", "businessOwnerId", "people", "manager@example.com"), opt("security_zone", "securityZone", "internal"), opt("description", "description", "First floor users")}},
	{Key: "ip_addresses", Title: "IP addresses", Object: "ip_address", KeyCol: "address", Parent: "routing_domain", Columns: []column{
		needRef("routing_domain", "routingDomainId", "routing_domains", "Corporate VRF"), kind(need("address", "address", "10.0.1.10"), ipCell), opt("hostname", "hostname", "printer-01"),
		opt("fqdn", "fqdn", "printer-01.example.com"), kind(opt("mac", "mac", "00:1b:44:11:3a:b7"), macCell), ref("device", "deviceId", "devices", "cmb-fw-01"),
		ref("application", "applicationId", "applications", "Payments API"), ref("business_service", "businessServiceId", "business_services", "Core Banking"),
		ref("owner_email", "ownerId", "people", "owner@example.com"), oneOf("allocation_state", "allocationState", "allocated", stateValues...), opt("notes", "notes", "Finance printer")}},
	{Key: "reserved_ranges", Title: "Reserved ranges", Object: "reserved_range", Entity: "/reserved-ranges", KeyCol: "range_start", Parent: "subnet", Columns: []column{
		needRef("routing_domain", "", "routing_domains", "Corporate VRF"), within(needRef("subnet", "subnetId", "subnets", "10.0.1.0/24"), "routing_domain"),
		kind(need("range_start", "rangeStart", "10.0.1.2"), ipCell), kind(need("range_end", "rangeEnd", "10.0.1.9"), ipCell), opt("reason", "reason", "Network equipment")}},
	{Key: "dhcp_scopes", Title: "DHCP scopes", Object: "dhcp_scope", Entity: "/dhcp-scopes", KeyCol: "range_start", Parent: "subnet", Columns: []column{
		needRef("routing_domain", "", "routing_domains", "Corporate VRF"), within(needRef("subnet", "subnetId", "subnets", "10.0.1.0/24"), "routing_domain"), opt("name", "name", "Users pool"),
		kind(need("range_start", "rangeStart", "10.0.1.100"), ipCell), kind(need("range_end", "rangeEnd", "10.0.1.200"), ipCell)}},
}
