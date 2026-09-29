package inspector

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Parsers for Linux /proc files. They live in an untagged file so they can be
// unit-tested on any OS; linux.go does the file I/O.

// procSocket is one listening socket from /proc/net/tcp or /proc/net/tcp6.
type procSocket struct {
	Addr  netip.Addr
	Port  int
	UID   int
	Inode uint64
}

const tcpListen = "0A" // TCP_LISTEN in /proc/net/tcp's "st" column

// parseProcNetTCP returns the listening sockets in a /proc/net/tcp or
// /proc/net/tcp6 file. Lines look like:
//
//	sl  local_address rem_address   st tx_queue:rx_queue tr:tm->when retrnsmt   uid  timeout inode
//	 0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 43210 ...
func parseProcNetTCP(data []byte) ([]procSocket, error) {
	var out []procSocket
	sc := bufio.NewScanner(bytes.NewReader(data))
	first := true
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if first { // header
			first = false
			continue
		}
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			return nil, fmt.Errorf("line %d: expected at least 10 fields, got %d", n, len(f))
		}
		if f[3] != tcpListen {
			continue
		}
		addr, port, err := parseProcAddr(f[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		uid, err := strconv.Atoi(f[7])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad uid %q", n, f[7])
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad inode %q", n, f[9])
		}
		out = append(out, procSocket{Addr: addr, Port: port, UID: uid, Inode: inode})
	}
	return out, sc.Err()
}

// parseProcAddr decodes "0100007F:0BB8" (IPv4) or a 32-hex-digit IPv6 address
// plus port. The address is printed as 32-bit words in host byte order
// (little-endian on every architecture portfind ships for); the port is plain
// big-endian hex.
func parseProcAddr(s string) (netip.Addr, int, error) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	if !ok {
		return netip.Addr{}, 0, fmt.Errorf("bad address %q", s)
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("bad port in %q", s)
	}
	raw, err := hex.DecodeString(hexAddr)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.Addr{}, 0, fmt.Errorf("bad address %q", s)
	}
	// Each 4-byte word was printed from a little-endian uint32; flip it back
	// into network order.
	for i := 0; i < len(raw); i += 4 {
		binary.BigEndian.PutUint32(raw[i:], binary.LittleEndian.Uint32(raw[i:]))
	}
	if len(raw) == 4 {
		return netip.AddrFrom4([4]byte(raw)), int(port), nil
	}
	return netip.AddrFrom16([16]byte(raw)).Unmap(), int(port), nil
}

// socketInode extracts the inode from an fd symlink target like "socket:[43210]".
func socketInode(link string) (uint64, bool) {
	rest, ok := strings.CutPrefix(link, "socket:[")
	if !ok || !strings.HasSuffix(rest, "]") {
		return 0, false
	}
	inode, err := strconv.ParseUint(strings.TrimSuffix(rest, "]"), 10, 64)
	return inode, err == nil
}

// procStat is the part of /proc/<pid>/stat portfind uses.
type procStat struct {
	Comm       string // executable name, truncated by the kernel to 15 bytes
	State      byte   // 'R', 'S', 'Z' (zombie), ...
	PPID       int
	StartTicks uint64 // start time in clock ticks since boot
}

// parseProcStat parses /proc/<pid>/stat. The comm field is in parentheses and
// may itself contain spaces or ')', so fields are counted from the last ')'.
func parseProcStat(data []byte) (procStat, error) {
	s := string(data)
	open, closing := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || closing < open {
		return procStat{}, fmt.Errorf("malformed stat: %q", truncateForError(s))
	}
	st := procStat{Comm: s[open+1 : closing]}
	// After ")": state(3) ppid(4) ... starttime(22); indices below are 0-based
	// from field 3.
	f := strings.Fields(s[closing+1:])
	if len(f) < 20 {
		return procStat{}, fmt.Errorf("stat has %d fields after comm, want at least 20", len(f))
	}
	st.State = f[0][0]
	var err error
	if st.PPID, err = strconv.Atoi(f[1]); err != nil {
		return procStat{}, fmt.Errorf("bad ppid %q", f[1])
	}
	if st.StartTicks, err = strconv.ParseUint(f[19], 10, 64); err != nil {
		return procStat{}, fmt.Errorf("bad starttime %q", f[19])
	}
	return st, nil
}

// parseBootTime reads "btime <unix seconds>" from /proc/stat.
func parseBootTime(data []byte) (int64, error) {
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		}
	}
	return 0, fmt.Errorf("no btime line in /proc/stat")
}

// ParseCmdline turns /proc/<pid>/cmdline (NUL-separated arguments) into a
// readable command line, quoting arguments that contain spaces.
func ParseCmdline(data []byte) string {
	args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			args[i] = strconv.Quote(a)
		}
	}
	return strings.TrimSpace(strings.Join(args, " "))
}

func truncateForError(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}
