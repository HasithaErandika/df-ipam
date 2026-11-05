package integrations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dfipam/api/internal/platform/safehttp"

	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// vCenter reads inventory through property retrieval only. It starts no tasks. The
// container view it creates lives in the session and changes nothing in vCenter.
type vCenter struct {
	url                *url.URL
	username, password string
	policy             safehttp.Policy
}

func newVCenter(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error) {
	base, err := policy.CheckURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return &vCenter{base.JoinPath("/sdk"), credentials["username"], credentials["password"], policy}, nil
}

func (v *vCenter) connect(ctx context.Context) (*vim25.Client, func(), error) {
	sc := soap.NewClient(v.url, false)
	tr := sc.DefaultTransport()
	tr.DialContext = v.policy.DialContext
	tr.Proxy = nil
	if v.policy.RootCAs != nil {
		tr.TLSClientConfig.RootCAs = v.policy.RootCAs
	}
	client, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, nil, cleanSOAP(err)
	}
	sessions := session.NewManager(client)
	if err := sessions.Login(ctx, url.UserPassword(v.username, v.password)); err != nil {
		return nil, nil, errors.New("vCenter refused the credentials")
	}
	return client, func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = sessions.Logout(ctx)
	}, nil
}

func cleanSOAP(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (v *vCenter) Test(ctx context.Context) error {
	_, done, err := v.connect(ctx)
	if err != nil {
		return err
	}
	done()
	return nil
}

func (v *vCenter) Collect(ctx context.Context) (*Result, error) {
	client, done, err := v.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return collectVCenter(ctx, client, time.Now())
}

func retrieve[T any](ctx context.Context, m *view.Manager, c *vim25.Client, kind string, props []string) ([]T, error) {
	cv, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{kind}, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	defer func() { _ = cv.Destroy(context.WithoutCancel(ctx)) }()
	var out []T
	if err := cv.Retrieve(ctx, []string{kind}, props, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	return out, nil
}

func guestFamily(guestID, fullName string) string {
	name := strings.ToLower(guestID + " " + fullName)
	switch {
	case strings.Contains(name, "windows") && strings.Contains(name, "server"), strings.Contains(name, "winnet"), strings.Contains(name, "windows20"):
		return "windows_server"
	case strings.Contains(name, "win"):
		return "windows"
	case strings.Contains(name, "linux"), strings.Contains(name, "ubuntu"), strings.Contains(name, "rhel"),
		strings.Contains(name, "centos"), strings.Contains(name, "debian"), strings.Contains(name, "sles"):
		return "linux"
	case strings.Contains(name, "darwin"):
		return "macos"
	}
	return "unknown"
}

// collectVCenter takes a connected client so tests can use the vCenter simulator.
func collectVCenter(ctx context.Context, c *vim25.Client, now time.Time) (*Result, error) {
	instance := c.ServiceContent.About.InstanceUuid
	if instance == "" {
		return nil, errors.New("vCenter reported no instance identifier")
	}
	m := view.NewManager(c)
	r := &Result{}

	hosts, err := retrieve[mo.HostSystem](ctx, m, c, "HostSystem", []string{"name", "summary.hardware", "summary.config.product", "parent"})
	if err != nil {
		return nil, err
	}
	for _, h := range hosts {
		d := Device{ExternalID: instance + ":" + h.Self.Value, Name: h.Name, Hostname: h.Name, DeviceType: "physical_server", OSFamily: "hypervisor"}
		if hw := h.Summary.Hardware; hw != nil {
			d.Vendor, d.Model = hw.Vendor, hw.Model
		}
		if p := h.Summary.Config.Product; p != nil {
			d.OSVersion = p.FullName
		}
		r.Devices = append(r.Devices, d)
	}

	networks := map[string]string{}
	if items, err := retrieve[mo.Network](ctx, m, c, "Network", []string{"name"}); err != nil {
		r.warn("NETWORKS_UNAVAILABLE", "", err)
	} else {
		for _, n := range items {
			networks[n.Self.Value] = n.Name
		}
	}
	vlans := map[string]int32{}
	if items, err := retrieve[mo.DistributedVirtualPortgroup](ctx, m, c, "DistributedVirtualPortgroup", []string{"name", "config.defaultPortConfig"}); err != nil {
		r.warn("PORTGROUPS_UNAVAILABLE", "", err)
	} else {
		for _, p := range items {
			networks[p.Self.Value] = p.Name
			if cfg, ok := p.Config.DefaultPortConfig.(*types.VMwareDVSPortSetting); ok {
				if id, ok := cfg.Vlan.(*types.VmwareDistributedVirtualSwitchVlanIdSpec); ok {
					vlans[p.Name] = id.VlanId
				}
			}
		}
	}

	vms, err := retrieve[mo.VirtualMachine](ctx, m, c, "VirtualMachine",
		[]string{"name", "config.uuid", "config.instanceUuid", "config.guestId", "config.guestFullName", "config.template", "config.hardware.device",
			"guest", "runtime.powerState", "runtime.host"})
	if err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.Config != nil && vm.Config.Template {
			continue
		}
		collectVM(r, vm, instance, networks, vlans, now)
	}
	return r, nil
}

func portgroupContext(name string) string { return "portgroup:" + name }

// collectVM records one VM. A guest IP counts as alive only if the VM is on and
// VMware Tools is running: anything else may be stale, so it stays as weak evidence.
func collectVM(r *Result, vm mo.VirtualMachine, instance string, networks map[string]string, vlans map[string]int32, now time.Time) {
	d := Device{ExternalID: instance + ":" + vm.Self.Value, Name: vm.Name, DeviceType: "virtual_machine", Vendor: "VMware"}
	var guestID, fullName string
	if vm.Config != nil {
		guestID, fullName = vm.Config.GuestId, vm.Config.GuestFullName
		d.OSVersion, d.Serial = fullName, vm.Config.Uuid
		if vm.Config.Hardware.Device != nil {
			for _, dev := range vm.Config.Hardware.Device {
				card, ok := dev.(types.BaseVirtualEthernetCard)
				if !ok {
					continue
				}
				nic := card.GetVirtualEthernetCard()
				name := fmt.Sprintf("nic-%d", nic.Key)
				if info := nic.DeviceInfo; info != nil {
					name = info.GetDescription().Label
				}
				d.Interfaces = append(d.Interfaces, Interface{Name: name, MAC: nic.MacAddress, Type: "vnic"})
			}
		}
	}
	d.OSFamily = guestFamily(guestID, fullName)
	power := string(vm.Runtime.PowerState)
	var tools string
	if vm.Guest != nil {
		d.Hostname = vm.Guest.HostName
		d.ManagementIP = vm.Guest.IpAddress
		tools = vm.Guest.ToolsRunningStatus
	}
	alive := vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn && tools == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning)
	r.Devices = append(r.Devices, d)
	if vm.Guest == nil {
		return
	}
	payload := func(network string) map[string]any {
		p := map[string]any{"vm": vm.Self.Value, "powerState": power, "toolsStatus": tools, "portgroup": network, "stale": !alive}
		if id, ok := vlans[network]; ok {
			p["vlan"] = id
		}
		return p
	}
	seen := map[string]bool{}
	for _, nic := range vm.Guest.Net {
		for _, ip := range nic.IpAddress {
			if seen[ip] {
				continue
			}
			seen[ip] = true
			r.Observations = append(r.Observations, Observation{Source: SourceVMware, Context: portgroupContext(nic.Network), Address: ip, MAC: nic.MacAddress,
				Hostname: vm.Guest.HostName, Alive: alive, ObservedAt: now, Payload: payload(nic.Network)})
		}
	}
	if ip := vm.Guest.IpAddress; ip != "" && !seen[ip] {
		r.Observations = append(r.Observations, Observation{Source: SourceVMware, Address: ip, Hostname: vm.Guest.HostName, Alive: alive, ObservedAt: now, Payload: payload("")})
	}
}
