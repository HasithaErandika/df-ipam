package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

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
		r.Devices = append(r.Devices, hostDevice(h, instance))
	}
	vlans := readVLANs(ctx, m, c, r)
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
		collectVM(r, vm, instance, vlans, now)
	}
	return r, nil
}

func hostDevice(h mo.HostSystem, instance string) Device {
	d := Device{ExternalID: instance + ":" + h.Self.Value, Name: h.Name, Hostname: h.Name, DeviceType: "physical_server", OSFamily: "hypervisor"}
	if hw := h.Summary.Hardware; hw != nil {
		d.Vendor, d.Model = hw.Vendor, hw.Model
	}
	if p := h.Summary.Config.Product; p != nil {
		d.OSVersion = p.FullName
	}
	return d
}

// readVLANs returns the VLAN of each distributed portgroup. It is optional evidence.
func readVLANs(ctx context.Context, m *view.Manager, c *vim25.Client, r *Result) map[string]int32 {
	vlans := map[string]int32{}
	items, err := retrieve[mo.DistributedVirtualPortgroup](ctx, m, c, "DistributedVirtualPortgroup", []string{"name", "config.defaultPortConfig"})
	if err != nil {
		r.warn("PORTGROUPS_UNAVAILABLE", "", err)
		return vlans
	}
	for _, p := range items {
		if cfg, ok := p.Config.DefaultPortConfig.(*types.VMwareDVSPortSetting); ok {
			if id, ok := cfg.Vlan.(*types.VmwareDistributedVirtualSwitchVlanIdSpec); ok {
				vlans[p.Name] = id.VlanId
			}
		}
	}
	return vlans
}

func vmInterfaces(devices []types.BaseVirtualDevice) []Interface {
	var out []Interface
	for _, dev := range devices {
		card, ok := dev.(types.BaseVirtualEthernetCard)
		if !ok {
			continue
		}
		nic := card.GetVirtualEthernetCard()
		name := fmt.Sprintf("nic-%d", nic.Key)
		if info := nic.DeviceInfo; info != nil {
			name = info.GetDescription().Label
		}
		out = append(out, Interface{Name: name, MAC: nic.MacAddress, Type: "vnic"})
	}
	return out
}

// collectVM records one VM. A guest IP counts as alive only if the VM is on and
// VMware Tools is running: anything else may be stale, so it stays as weak evidence.
func collectVM(r *Result, vm mo.VirtualMachine, instance string, vlans map[string]int32, now time.Time) {
	d := Device{ExternalID: instance + ":" + vm.Self.Value, Name: vm.Name, DeviceType: "virtual_machine", Vendor: "VMware"}
	var guestID, fullName string
	if vm.Config != nil {
		guestID, fullName = vm.Config.GuestId, vm.Config.GuestFullName
		d.OSVersion, d.Serial = fullName, vm.Config.Uuid
		d.Interfaces = vmInterfaces(vm.Config.Hardware.Device)
	}
	d.OSFamily = guestFamily(guestID, fullName)
	if vm.Guest != nil {
		d.Hostname = vm.Guest.HostName
		d.ManagementIP = vm.Guest.IpAddress
	}
	r.Devices = append(r.Devices, d)
	if vm.Guest != nil {
		observeGuest(r, vm, vlans, now)
	}
}

func observeGuest(r *Result, vm mo.VirtualMachine, vlans map[string]int32, now time.Time) {
	tools := vm.Guest.ToolsRunningStatus
	alive := vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn && tools == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning)
	payload := func(network string) map[string]any {
		p := map[string]any{"vm": vm.Self.Value, "powerState": string(vm.Runtime.PowerState), "toolsStatus": tools, "portgroup": network, "stale": !alive}
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
