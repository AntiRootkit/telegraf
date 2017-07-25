package vsphere

import (
	"context"
	"fmt"
	"net/url"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type VSphere struct {
	Server          string `json:"server"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Insecure        bool   `json:"insecure"`
	Hosts           []string   `json:"hosts"`
	Datastores      []string   `json:"datastores"`
	VirtualMachines []string   `json:"virtual_machines"`
}

var sampleConfig = `
  ## FQDN or an IP of a vCenter Server or ESXi host
  server = "vcenter.domain.com"

  ## A vSphere/ESX user
  ## must have System.View and Performance.ModifyIntervals privileges
  username = "root"

  ## Password
  password = "vmware"

  ## Do not validate server's TLS certificate
  # insecure =  true

  ## Host name patterns
  # hosts = ["*"]

  ## Datastore name patterns
  # datastores = ["*"]

  ## Virtual machine name patterns
  # virtual_machines = ["*"]
`

func (v *VSphere) Description() string {
	return "Collect metrics from VMware vSphere"
}

func (v *VSphere) SampleConfig() string {
	return sampleConfig
}

func (v *VSphere) gatherDatastoreMetrics(acc telegraf.Accumulator, ctx context.Context, c *govmomi.Client, pc *property.Collector, dss []*object.Datastore) error {
	// Convert datastores into list of references
	var refs []types.ManagedObjectReference
	for _, ds := range dss {
		refs = append(refs, ds.Reference())
	}

	// Retrieve summary property for all datastores
	var dst []mo.Datastore
	err := pc.Retrieve(ctx, refs, []string{"summary"}, &dst)
	if err != nil {
		return (err)
	}

	for _, ds := range dst {
		records := make(map[string]interface{})
		tags := make(map[string]string)

		tags["name"] = ds.Summary.Name

		records["type"] = ds.Summary.Type
		records["capacity"] = ds.Summary.Capacity
		records["free_space"] = ds.Summary.FreeSpace
		records["uncommitted_space"] = ds.Summary.Uncommitted

		acc.AddFields("datastore", records, tags)
	}

	return nil
}

func (v *VSphere) gatherVMMetrics(acc telegraf.Accumulator, ctx context.Context, c *govmomi.Client, pc *property.Collector, vms []*object.VirtualMachine) error {
	// Convert datastores into list of references
	var refs []types.ManagedObjectReference
	for _, vm := range vms {
		refs = append(refs, vm.Reference())
	}

	// Retrieve name property for all vms
	var vmt []mo.VirtualMachine
	err := pc.Retrieve(ctx, refs, []string{"name", "config", "summary"}, &vmt)
	if err != nil {
		return (err)
	}

	for _, vm := range vmt {

		records := make(map[string]interface{})
		tags := make(map[string]string)

		tags["name"] = vm.Name
		tags["hostname"] = vm.Summary.Guest.HostName

		records["guest_os_name"] = vm.Config.GuestFullName
		records["guest_os_id"] = vm.Config.GuestId
		records["ip_address"] = vm.Summary.Guest.IpAddress

		records["connection_state"] = string(vm.Summary.Runtime.ConnectionState)
		records["health_status"] = string(vm.Summary.OverallStatus)
		records["guest_tools_running"] = vm.Summary.Guest.ToolsRunningStatus

		records["cpu_sockets"] = vm.Config.Hardware.NumCPU
		records["cpu_cores_per_socket"] = vm.Config.Hardware.NumCoresPerSocket
		records["cpu_entitlement"] = vm.Summary.Runtime.MaxCpuUsage
		records["cpu_usage"] = vm.Summary.QuickStats.OverallCpuUsage
		records["cpu_demand"] = vm.Summary.QuickStats.OverallCpuDemand

		records["memory_granted"] = vm.Config.Hardware.MemoryMB
		records["memory_entitlement"] = vm.Summary.Runtime.MaxMemoryUsage
		records["memory_host_consumed"] = vm.Summary.QuickStats.HostMemoryUsage
		records["memory_guest_active"] = vm.Summary.QuickStats.GuestMemoryUsage
		records["memory_swapped"] = vm.Summary.QuickStats.SwappedMemory
		records["memory_ballooned"] = vm.Summary.QuickStats.BalloonedMemory

		records["uptime"] = vm.Summary.QuickStats.UptimeSeconds
		records["storage_committed"] = vm.Summary.Storage.Committed
		records["storage_uncommitted"] = vm.Summary.Storage.Uncommitted

		acc.AddFields("virtual_machine", records, tags)
	}

	return nil
}

func (v *VSphere) gatherHostMetrics(acc telegraf.Accumulator, ctx context.Context, c *govmomi.Client, pc *property.Collector, hosts []*object.HostSystem) error {
	var refs []types.ManagedObjectReference
	for _, host := range hosts {
		refs = append(refs, host.Reference())
	}

	var hostObjects []mo.HostSystem
	err := pc.Retrieve(ctx, refs, []string{"name", "summary"}, &hostObjects)
	if err != nil {
		return (err)
	}

	for _, host := range hostObjects {

		records := make(map[string]interface{})
		tags := make(map[string]string)

		tags["name"] = host.Name

		records["connection_state"] = host.Summary.Runtime.ConnectionState
		records["health_status"] = string(host.Summary.OverallStatus)

		records["cpu_cores"] = host.Summary.Hardware.NumCpuCores
		records["cpu_speed"] = host.Summary.Hardware.CpuMhz
		records["cpu_usage"] = host.Summary.QuickStats.OverallCpuUsage

		records["memory_granted"] = host.Summary.Hardware.MemorySize / 1024 / 1024
		records["memory_usage"] = host.Summary.QuickStats.OverallMemoryUsage


		acc.AddFields("host", records, tags)
	}

	return nil
}

func (v *VSphere) Gather(acc telegraf.Accumulator) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse URL from string
	u, err := url.Parse(fmt.Sprintf("https://%s:%s@%s/sdk", v.Username, v.Password, v.Server))
	if err != nil {
		return err
	}

	// Connect and log in to ESX or vCenter
	c, err := govmomi.NewClient(ctx, u, v.Insecure)
	if err != nil {
		return err
	}
	f := find.NewFinder(c.Client, true)

	// Find one and only datacenter
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		return err
	}

	// Make future calls local to this datacenter
	f.SetDatacenter(dc)

	pc := property.DefaultCollector(c.Client)

	for _, ds := range v.Datastores {
		dss, err := f.DatastoreList(ctx, ds)
		if err != nil {
			return err
		}
		err = v.gatherDatastoreMetrics(acc, ctx, c, pc, dss)
		if err != nil {
			return err
		}
	}

	for _, vm := range v.VirtualMachines {
		vms, err := f.VirtualMachineList(ctx, vm)
		if err != nil {
			return err
		}
		err = v.gatherVMMetrics(acc, ctx, c, pc, vms)
		if err != nil {
			return err
		}
	}

	for _, hostnamePattern := range v.Hosts {
		hosts, err := f.HostSystemList(ctx, hostnamePattern)
		if err != nil {
			return err
		}
		err = v.gatherHostMetrics(acc, ctx, c, pc, hosts)
		if err != nil {
			return err
		}
	}

	return nil
}

func init() {
	inputs.Add("vsphere", func() telegraf.Input { return &VSphere{} })
}
