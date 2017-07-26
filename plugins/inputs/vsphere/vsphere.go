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
	"sync"
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

func (v *VSphere) Gather(acc telegraf.Accumulator) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse URL from string
	u, err := url.Parse(fmt.Sprintf("https://%s:%s@%s/sdk", v.Username, v.Password, v.Server))
	if err != nil {
		return err
	}

	// Connect and log in to ESX or vCenter
	client, err := govmomi.NewClient(ctx, u, v.Insecure)
	if err != nil {
		return err
	}
	finder := find.NewFinder(client.Client, true)

	// Find one and only datacenter
	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return err
	}
	finder.SetDatacenter(dc)

	var wg sync.WaitGroup

	for _, name := range v.Hosts {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			hosts, err := finder.HostSystemList(ctx, name)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read host list for '%s': %s", name, err))
				return
			}


			err = v.gatherHostMetrics(acc, ctx, client, hosts)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read host properties for '%s': %s", name, err))
				return
			}
		}(name)
	}

	for _, name := range v.Datastores {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			datastores, err := finder.DatastoreList(ctx, name)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read datastore list for '%s': %s", name, err))
				return
			}
			err = v.gatherDatastoreMetrics(acc, ctx, client, datastores)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read datastore properties for '%s': %s", name, err))
				return
			}
		}(name)
	}

	for _, name := range v.VirtualMachines {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			vms, err := finder.VirtualMachineList(ctx, name)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read vm list for '%s': %s", name, err))
				return
			}
			err = v.gatherVMMetrics(acc, ctx, client, vms)
			if err != nil {
				acc.AddError(fmt.Errorf("Cannot read vm properties for '%s': %s", name, err))
				return
			}
		}(name)
	}

	wg.Wait()
	return nil
}

func (v *VSphere) gatherHostMetrics(acc telegraf.Accumulator, ctx context.Context, client *govmomi.Client, hosts []*object.HostSystem) error {
	var refs []types.ManagedObjectReference
	for _, obj := range hosts {
		refs = append(refs, obj.Reference())
	}

	collector := property.DefaultCollector(client.Client)
	var results []mo.HostSystem
	err := collector.Retrieve(ctx, refs, []string{"name", "summary"}, &results)
	if err != nil {
		return err
	}

	for _, host := range results {

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

func (v *VSphere) gatherDatastoreMetrics(acc telegraf.Accumulator, ctx context.Context, client *govmomi.Client, datastores []*object.Datastore) error {
	// Convert datastores into list of references
	var refs []types.ManagedObjectReference
	for _, obj := range datastores {
		refs = append(refs, obj.Reference())
	}

	collector := property.DefaultCollector(client.Client)
	var results []mo.Datastore
	err := collector.Retrieve(ctx, refs, []string{"summary"}, &results)
	if err != nil {
		return err
	}

	for _, datastore := range results {
		records := make(map[string]interface{})
		tags := make(map[string]string)

		tags["name"] = datastore.Summary.Name

		records["type"] = datastore.Summary.Type
		records["health_status"] = string(datastore.OverallStatus)

		records["capacity"] = datastore.Summary.Capacity
		records["free_space"] = datastore.Summary.FreeSpace
		records["uncommitted_space"] = datastore.Summary.Uncommitted

		acc.AddFields("datastore", records, tags)
	}

	return nil
}

func (v *VSphere) gatherVMMetrics(acc telegraf.Accumulator, ctx context.Context, client *govmomi.Client, vms []*object.VirtualMachine) error {
	var refs []types.ManagedObjectReference
	for _, obj := range vms {
		refs = append(refs, obj.Reference())
	}

	collector := property.DefaultCollector(client.Client)
	var results []mo.VirtualMachine
	err := collector.Retrieve(ctx, refs, []string{"name", "config", "summary"}, &results)
	if err != nil {
		return err
	}

	for _, vm := range results {

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

		records["storage_committed"] = vm.Summary.Storage.Committed
		records["storage_uncommitted"] = vm.Summary.Storage.Uncommitted

		acc.AddFields("virtual_machine", records, tags)
	}

	return nil
}

func init() {
	inputs.Add("vsphere", func() telegraf.Input { return &VSphere{} })
}
