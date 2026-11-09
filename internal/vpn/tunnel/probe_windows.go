//go:build windows

package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported reports whether tunnel probing is implemented on this platform.
func Supported() bool { return true }

// Socket tables come from the IP Helper API rather than netstat.exe: netstat's
// state column is localized, and spawning it on every poll is slow.
var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	tcpTableOwnerPIDAll = 5 // TCP_TABLE_OWNER_PID_ALL
	udpTableOwnerPID    = 1 // UDP_TABLE_OWNER_PID
	mibTCPStateEstab    = 5 // MIB_TCP_STATE_ESTAB
)

type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

type mibUDPRowOwnerPID struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPID uint32
}

// vpnAdapterHints match the descriptions of VPN virtual adapters whose address
// is worth reporting as the tunnel IP. Tailscale is deliberately absent: it is
// read through its own CLI and would otherwise blank the address of a real
// VPN running beside it.
var vpnAdapterHints = []string{
	"juniper networks virtual adapter",
	"pulse secure",
	"ivanti",
	"fortinet",
	"forticlient",
	"forcepoint",
	"stonesoft",
}

func collect(_ context.Context) *snapshot {
	names := processNames()
	return &snapshot{
		captured: time.Now(),
		tcp:      tcpTable(names),
		udp:      udpTable(names),
		tunnels:  listTunnelAddresses(),
	}
}

// tcpTable renders IPv4 TCP connections as lines the shared matcher reads:
//
//	tcp4 10.0.0.2:50123 1.1.254.50:443 ESTABLISHED 4321 dsAccessService.exe
func tcpTable(names map[uint32]string) string {
	buf := extendedTable(procGetExtendedTcpTable, tcpTableOwnerPIDAll)
	if len(buf) < 4 {
		return ""
	}
	count := binary.LittleEndian.Uint32(buf)
	rowSize := unsafe.Sizeof(mibTCPRowOwnerPID{})
	var b strings.Builder
	for i := uint32(0); i < count; i++ {
		off := 4 + uintptr(i)*rowSize
		if off+rowSize > uintptr(len(buf)) {
			break
		}
		row := (*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[off]))
		state := "OTHER"
		if row.State == mibTCPStateEstab {
			state = "ESTABLISHED"
		}
		fmt.Fprintf(&b, "tcp4 %s:%d %s:%d %s %d %s\n",
			ipv4(row.LocalAddr), port(row.LocalPort),
			ipv4(row.RemoteAddr), port(row.RemotePort),
			state, row.OwningPID, names[row.OwningPID])
	}
	return b.String()
}

func udpTable(names map[uint32]string) string {
	buf := extendedTable(procGetExtendedUdpTable, udpTableOwnerPID)
	if len(buf) < 4 {
		return ""
	}
	count := binary.LittleEndian.Uint32(buf)
	rowSize := unsafe.Sizeof(mibUDPRowOwnerPID{})
	var b strings.Builder
	for i := uint32(0); i < count; i++ {
		off := 4 + uintptr(i)*rowSize
		if off+rowSize > uintptr(len(buf)) {
			break
		}
		row := (*mibUDPRowOwnerPID)(unsafe.Pointer(&buf[off]))
		fmt.Fprintf(&b, "udp4 %s:%d *:* %d %s\n",
			ipv4(row.LocalAddr), port(row.LocalPort), row.OwningPID, names[row.OwningPID])
	}
	return b.String()
}

// extendedTable calls GetExtendedTcpTable / GetExtendedUdpTable for AF_INET,
// growing the buffer until the table fits.
func extendedTable(proc *windows.LazyProc, class uintptr) []byte {
	if proc.Find() != nil {
		return nil
	}
	size := uint32(16 << 10)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, size)
		ret, _, _ := proc.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
			0,
			uintptr(windows.AF_INET),
			class,
			0,
		)
		switch windows.Errno(ret) {
		case 0:
			return buf[:size]
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4 << 10
			continue
		default:
			return nil
		}
	}
	return nil
}

func ipv4(addr uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], addr)
	return net.IP(b[:]).String()
}

// port converts the network-order port held in the low 16 bits of dw.
func port(dw uint32) uint16 {
	return uint16(dw>>8&0xff) | uint16(dw&0xff)<<8
}

// processNames maps PIDs to image names. Listing processes needs no admin
// rights, unlike netstat -b.
func processNames() map[uint32]string {
	names := make(map[uint32]string)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return names
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		names[entry.ProcessID] = windows.UTF16ToString(entry.ExeFile[:])
	}
	return names
}

// listTunnelAddresses returns the IPv4 addresses of VPN virtual adapters that
// are up.
func listTunnelAddresses() []tunnelAddr {
	size := uint32(15 << 10)
	var buf []byte
	for attempt := 0; attempt < 4; attempt++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_INET,
			windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
		buf = nil
	}
	if buf == nil {
		return nil
	}

	var out []tunnelAddr
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		desc := windows.UTF16PtrToString(a.Description)
		name := windows.UTF16PtrToString(a.FriendlyName)
		if !isVPNAdapter(desc + " " + name) {
			continue
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			ip := u.Address.IP()
			if ip == nil || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, tunnelAddr{ip: ip.String(), iface: name, desc: desc})
		}
	}
	return out
}

func isVPNAdapter(label string) bool {
	label = strings.ToLower(label)
	for _, hint := range vpnAdapterHints {
		if strings.Contains(label, hint) {
			return true
		}
	}
	return false
}
