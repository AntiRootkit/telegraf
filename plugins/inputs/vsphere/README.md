# Telegraf Input Plugin: VMware vSphere

This plugin collects metrics from vCenter servers or standalone vSphere Hypervisor (ESXi) hosts.

- Hosts 
    - Health status
    - CPU usage
    - Memory usage
- Datastores 
    - Space usage
- Virtual machines
    - Health status
    - Guest OS details
    - CPU usage
    - Memory usage
    - Storage usage

### Configuration:

```toml
# Collect metrics from VMware vSphere
[[inputs.vsphere]]
  ## FQDN or an IP of a vCenter Server or ESXi host
  server = "vcenter.domain.com"

  ## A vSphere/ESX user
  ## must have System.View privilege
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
```

### Measurements & Fields:

- host
    - connection_state (string)
    - health_status (string)
    - cpu_cores (integer)
    - cpu_speed (integer, MHz per core)
    - cpu_usage (integer, MHz total)
    - memory_granted (integer, MB)
    - memory_usage (integer, MB)
- datastore
    - type (string)
    - capacity (integer, bytes)
    - free_space (integer, bytes)
    - uncommitted_space (integer, bytes)
- virtual_machine
    - guest_os_name (string)
    - guest_os_id (string)
    - ip_address (string)
    - connection_state (string)
    - health_status (string)
    - guest_tools_running (bool)
    - cpu_sockets (integer)
    - cpu_cores_per_socket  (integer)
    - cpu_entitlement (integer, MHz)
    - cpu_usage (integer, MHz)
    - cpu_demand (integer, MHz)
    - memory_granted (integer, MB)
    - memory_entitlement (integer, MB)
    - memory_host_consumed (integer, MB)
    - memory_guest_active (integer, MB)
    - memory_swapped (integer, MB)
    - memory_ballooned (integer, MB)
    - storage_committed (integer, bytes)
    - storage_uncommitted (integer, bytes)

### Tags:

- All measurements have the following tags:
    - name
- `virtual_machine` has the following tags:
    - hostname

<!---
### Sample Queries:

```
SELECT mean("host_mem_usage") FROM "vm_metrics" WHERE "name" =~ /^$VM$/ AND $timeFilter GROUP BY time($interval) fill(null) // Memory used
SELECT mean("max_mem_usage") FROM "vm_metrics" WHERE "name" =~ /^$VM$/ AND $timeFilter GROUP BY time($interval) fill(null) // Max memory
```
--->

### Example Output:

```
$ ./telegraf -config telegraf.conf -input-filter vsphere -test
host,name=esxi1.domain.com cpu_speed=2693i,cpu_usage=25134i,memory_granted=393137i,memory_usage=376990i,connection_state="connected",health_status="green",cpu_cores=16i 1501106147000000000
datastore,name=ds1 type="VMFS",capacity=9895336214528i,free_space=1510909935616i,uncommitted_space=19212208096956i 1501107520000000000
virtual_machine,name=vm1,hostname=vm1.domain.com ip_address="192.168.1.2",cpu_demand=43i,memory_host_consumed=8150i,memory_ballooned=0i,memory_entitlement=8192i,memory_swapped=0i,guest_os_id="ubuntu64Guest",connection_state="connected",health_status="green",guest_tools_running="guestToolsRunning",cpu_entitlement=2194i,cpu_sockets=1i,storage_uncommitted=4786750081i,storage_committed=57669734481i,guest_os_name="Ubuntu Linux (64-bit)",cpu_cores_per_socket=1i,cpu_usage=43i,memory_granted=8192i,memory_guest_active=737i 1501106028000000000
```
