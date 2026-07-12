//go:build windows

package agent

import (
	"net"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"connect/internal/rendezvous"

	"golang.org/x/sys/windows/registry"
)

func fillPlatformInventory(inv *rendezvous.HostInventory) {
	fillWindowsIdentity(inv)
	fillWindowsOS(inv)
	fillWindowsHardware(inv)
	fillWindowsMetrics(inv)
	fillWindowsNetwork(inv)
}

func refreshLiveInventory(inv *rendezvous.HostInventory) {
	if inv == nil {
		return
	}
	fillWindowsMetrics(inv)
}

func fillWindowsIdentity(inv *rendezvous.HostInventory) {
	if u, err := user.Current(); err == nil && u != nil {
		if i := strings.LastIndex(u.Username, `\`); i >= 0 {
			inv.Domain = u.Username[:i]
			inv.User = u.Username[i+1:]
		} else {
			inv.User = u.Username
		}
	}
	if inv.User == "" {
		inv.User = os.Getenv("USERNAME")
	}
	if inv.Domain == "" {
		inv.Domain = os.Getenv("USERDOMAIN")
	}
	if dns := strings.TrimSpace(os.Getenv("USERDNSDOMAIN")); dns != "" && inv.FQDN != "" && !strings.Contains(inv.FQDN, ".") {
		inv.FQDN = inv.FQDN + "." + strings.ToLower(dns)
	}
}

func fillWindowsOS(inv *rendezvous.HostInventory) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		inv.OS = "Windows"
		return
	}
	defer k.Close()
	product, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuild")
	ubr, _, _ := k.GetIntegerValue("UBR")
	if product != "" {
		inv.OS = product
	} else {
		inv.OS = "Windows"
	}
	switch {
	case display != "" && build != "":
		if ubr > 0 {
			inv.OSVersion = display + " (build " + build + "." + strconv.FormatUint(ubr, 10) + ")"
		} else {
			inv.OSVersion = display + " (build " + build + ")"
		}
	case build != "":
		inv.OSVersion = "build " + build
	case display != "":
		inv.OSVersion = display
	}
}

func fillWindowsHardware(inv *rendezvous.HostInventory) {
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\SystemInformation`, registry.QUERY_VALUE); err == nil {
		inv.Manufacturer, _, _ = k.GetStringValue("SystemManufacturer")
		inv.Model, _, _ = k.GetStringValue("SystemProductName")
		if bios, _, err := k.GetStringValue("BIOSVersion"); err == nil && bios != "" {
			inv.BIOS = bios
		} else if bios, _, err := k.GetStringValue("SystemBIOSVersion"); err == nil {
			inv.BIOS = bios
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		if inv.Manufacturer == "" {
			inv.Manufacturer, _, _ = k.GetStringValue("SystemManufacturer")
		}
		if inv.Model == "" {
			inv.Model, _, _ = k.GetStringValue("SystemProductName")
		}
		if inv.BIOS == "" {
			inv.BIOS, _, _ = k.GetStringValue("BIOSVersion")
		}
		if sn, _, err := k.GetStringValue("SystemSerialNumber"); err == nil {
			inv.Serial = sn
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE); err == nil {
		inv.UUID, _, _ = k.GetStringValue("MachineGuid")
		k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		inv.CPU, _, _ = k.GetStringValue("ProcessorNameString")
		inv.CPU = strings.TrimSpace(inv.CPU)
		k.Close()
	}
	inv.Monitors = monitorCount()
}

func fillWindowsMetrics(inv *rendezvous.HostInventory) {
	used, total, pct, pageTot, pageAvail := sampleMemoryFull()
	inv.CPUPct = sampleCPU()
	inv.CPUCores = logicalCPUs()
	inv.MemUsedGB = round1(used)
	inv.MemTotalGB = round1(total)
	inv.MemAvailGB = round1(total - used)
	inv.MemPct = pct
	inv.PagefileTotGB = round1(pageTot)
	inv.PagefileAvailGB = round1(pageAvail)
	free, diskTot := sampleDiskC()
	inv.DiskVol = `C:`
	inv.DiskFreeGB = round1(free)
	inv.DiskTotalGB = round1(diskTot)
	inv.UptimeSec = sampleUptime()
}

func fillWindowsNetwork(inv *rendezvous.HostInventory) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		var v4, v6 string
		for _, addr := range addrs {
			ip, ok := addr.(*net.IPNet)
			if !ok || ip.IP.IsLoopback() {
				continue
			}
			if x := ip.IP.To4(); x != nil {
				if v4 == "" {
					v4 = x.String()
				}
			} else if ip.IP.To16() != nil && v6 == "" {
				v6 = ip.IP.String()
			}
		}
		if v4 == "" && v6 == "" {
			continue
		}
		inv.Adapter = iface.Name
		inv.IPv4 = v4
		inv.IPv6 = v6
		if len(iface.HardwareAddr) > 0 {
			inv.MAC = strings.ToUpper(iface.HardwareAddr.String())
		}
		return
	}
}

func monitorCount() int {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("GetSystemMetrics")
	n, _, _ := proc.Call(80) // SM_CMONITORS
	if n == 0 {
		return 1
	}
	return int(n)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
