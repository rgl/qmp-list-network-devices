# About

Lists a QEMU VM network devices using its QEMU Machine Protocol (QMP) monitor socket.

This was mainly created to retrieve the VM IP address of a specific QEMU network device.

This was created in the context of [adding bridge networking into packer](https://github.com/hashicorp/packer/issues/9156) to make it able to [install an ESXi 7.0 VM](https://github.com/rgl/esxi-vagrant).

## Usage

Launch the VM:

```bash
qemu-img create -f qcow2 test-fiddle.qcow2 40G
qemu-system-x86_64 \
  -name 'ESXi Test Fiddle' \
  -machine pc,accel=kvm \
  -cpu host \
  -m 4G \
  -smp cores=4 \
  -k pt \
  -qmp unix:test-fiddle.socket,server,nowait \
  -netdev bridge,id=net0,br=virbr0 \
  -device vmxnet3,netdev=net0,mac=52:54:00:12:34:56 \
  -drive if=ide,media=disk,discard=unmap,format=qcow2,cache=unsafe,file=test-fiddle.qcow2 \
  -drive if=ide,media=cdrom,file=VMware-VMvisor-Installer-7.0.0-15843807.x86_64.iso
```

**NB** make sure you use an unique mac address within your network because qemu will not do that.

Then execute this application:

```bash
./qmp-list-network-devices test-fiddle.socket
```

And it should return something alike:

```plain
Name,Type,MacAddress,IpAddress,Path
net0,vmxnet3,52:54:00:12:34:56,192.168.121.111,/machine/peripheral-anon/device[0]
```

## References

* https://github.com/qemu/qemu/blob/v4.2.0/qapi/net.json
* https://github.com/qemu/qemu/blob/v4.2.0/scripts/qmp/qom-tree
* https://github.com/qemu/qemu/tree/v4.2.0/python/qemu
* [qemu_macaddr_default_if_unset](https://github.com/qemu/qemu/blob/v4.2.0/net/net.c#L179-L200)
* https://github.com/torvalds/linux/blob/v5.4/include/uapi/linux/if_arp.h#L132
