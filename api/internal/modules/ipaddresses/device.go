package ipaddresses

import (
	"net"
	"time"
)

// DeviceInfo is what DF-IPAM legitimately knows about the device behind an
// address: its inventory records and what approved read-only sources
// reported. Nothing here comes from probing the device, and nothing is
// guessed: the operating system appears only when a record states it.
type DeviceInfo struct {
	Address     string          `json:"address"`
	Device      *DeviceRecord   `json:"device"`
	Interface   *InterfaceInfo  `json:"interface"`
	MAC         *MACInfo        `json:"mac"`
	DNS         []DNSName       `json:"dnsRecords"`
	DHCP        []DHCPLease     `json:"dhcpReservations"`
	Sources     []SourceSummary `json:"sources"`
	HasEvidence bool            `json:"hasEvidence"`
}

type DeviceRecord struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	DeviceType      string     `json:"deviceType"`
	Vendor          string     `json:"vendor"`
	Model           string     `json:"model"`
	SerialNumber    *string    `json:"serialNumber"`
	OSVersion       string     `json:"osVersion"`
	OSFamily        string     `json:"osFamily"`
	ManagementIP    *string    `json:"managementIp"`
	Status          string     `json:"status"`
	Sensitivity     string     `json:"sensitivity"`
	SiteName        *string    `json:"siteName"`
	SourceSystem    string     `json:"sourceSystem"`
	LastDiscovered  *time.Time `json:"lastDiscovered"`
	LastVerified    *time.Time `json:"lastVerified"`
	OtherAddresses  int64      `json:"otherAddresses"`
	BusinessService *string    `json:"businessService"`
}

type InterfaceInfo struct {
	Name         string     `json:"name"`
	MAC          *string    `json:"mac"`
	Type         string     `json:"interfaceType"`
	AdminStatus  string     `json:"adminStatus"`
	OperStatus   string     `json:"operStatus"`
	VLAN         *string    `json:"vlan"`
	VDOM         *string    `json:"vdom"`
	Source       string     `json:"source"`
	LastObserved *time.Time `json:"lastObserved"`
}

// MACInfo is read from the address itself, so it needs no lookup. A locally
// administered address is usually randomised or virtual.
type MACInfo struct {
	Address             string `json:"address"`
	LocallyAdministered bool   `json:"locallyAdministered"`
	Multicast           bool   `json:"multicast"`
	OUI                 string `json:"oui"`
}

type DNSName struct {
	Name       string     `json:"name"`
	RecordType string     `json:"recordType"`
	Value      string     `json:"value"`
	Source     *string    `json:"source"`
	LastSeen   *time.Time `json:"lastSeen"`
}

type DHCPLease struct {
	Scope    string     `json:"scope"`
	MAC      *string    `json:"mac"`
	Hostname string     `json:"hostname"`
	Source   *string    `json:"source"`
	LastSeen *time.Time `json:"lastSeen"`
}

type SourceSummary struct {
	Source       string    `json:"source"`
	Reliability  int16     `json:"reliability"`
	Observations int64     `json:"observations"`
	LastSeen     time.Time `json:"lastSeen"`
}

// DescribeMAC reads the flags in the first octet of a MAC address.
func DescribeMAC(value string) *MACInfo {
	hw, err := net.ParseMAC(value)
	if err != nil || len(hw) < 3 {
		return nil
	}
	return &MACInfo{Address: hw.String(), LocallyAdministered: hw[0]&0x02 != 0, Multicast: hw[0]&0x01 != 0,
		OUI: net.HardwareAddr(hw[:3]).String()}
}
