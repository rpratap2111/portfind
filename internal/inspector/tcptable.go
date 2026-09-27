package inspector

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
)

// listener is one raw row from an OS TCP listener table, before the owning
// process has been resolved.
type listener struct {
	Addr netip.Addr
	Port int
	PID  int
}

// Row sizes of the Windows MIB_TCPROW_OWNER_PID / MIB_TCP6ROW_OWNER_PID
// structs. Both tables start with a DWORD row count followed immediately by
// the rows (4-byte alignment, so no padding).
const (
	tableHeaderSize = 4
	tcp4RowSize     = 24 // state, localAddr, localPort, remoteAddr, remotePort, pid
	tcp6RowSize     = 56 // localAddr[16], scope, localPort, remoteAddr[16], scope, remotePort, state, pid
)

// parseTCP4Table decodes a MIB_TCPTABLE_OWNER_PID buffer.
func parseTCP4Table(buf []byte) ([]listener, error) {
	return parseTable(buf, tcp4RowSize, func(row []byte) listener {
		return listener{
			Addr: netip.AddrFrom4([4]byte(row[4:8])),
			Port: portFromNetworkOrder(row[8:10]),
			PID:  int(binary.LittleEndian.Uint32(row[20:24])),
		}
	})
}

// parseTCP6Table decodes a MIB_TCP6TABLE_OWNER_PID buffer.
func parseTCP6Table(buf []byte) ([]listener, error) {
	return parseTable(buf, tcp6RowSize, func(row []byte) listener {
		return listener{
			Addr: netip.AddrFrom16([16]byte(row[0:16])),
			Port: portFromNetworkOrder(row[20:22]),
			PID:  int(binary.LittleEndian.Uint32(row[52:56])),
		}
	})
}

func parseTable(buf []byte, rowSize int, decode func(row []byte) listener) ([]listener, error) {
	if len(buf) < tableHeaderSize {
		return nil, fmt.Errorf("tcp table buffer too short: %d bytes", len(buf))
	}
	n := int(binary.LittleEndian.Uint32(buf[:tableHeaderSize]))
	if n > (len(buf)-tableHeaderSize)/rowSize {
		return nil, fmt.Errorf("tcp table claims %d rows of %d bytes but buffer holds only %d bytes",
			n, rowSize, len(buf))
	}
	out := make([]listener, 0, n)
	for i := 0; i < n; i++ {
		off := tableHeaderSize + i*rowSize
		out = append(out, decode(buf[off:off+rowSize]))
	}
	return out, nil
}

// portFromNetworkOrder reads a port stored in network (big-endian) byte order
// in the low two bytes of a DWORD, as Windows does for dwLocalPort.
func portFromNetworkOrder(b []byte) int {
	return int(binary.BigEndian.Uint16(b))
}

// dedupeListeners collapses rows that share a (port, PID) pair — e.g. a server
// bound on both 0.0.0.0 and [::] — and returns them sorted by port, then PID.
func dedupeListeners(in []listener) []listener {
	type key struct{ port, pid int }
	seen := make(map[key]bool, len(in))
	out := make([]listener, 0, len(in))
	for _, l := range in {
		k := key{l.Port, l.PID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].PID < out[j].PID
	})
	return out
}

func hasListener(listeners []listener, port, pid int) bool {
	for _, l := range listeners {
		if l.Port == port && l.PID == pid {
			return true
		}
	}
	return false
}
