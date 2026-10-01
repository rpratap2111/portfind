package inspector

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Parsers for the macOS kernel structures portfind reads through libproc and
// sysctl. They live in an untagged file so they can be unit-tested on any OS;
// darwin.go makes the calls. Offsets come from <sys/proc_info.h> and
// <netinet/in_pcb.h>, and are the same on Intel and Apple silicon (both
// little-endian).

// proc_pidinfo and proc_pidfdinfo flavors, and the buffers they fill.
const (
	procPidListFDs       = 1 // PROC_PIDLISTFDS: array of proc_fdinfo
	procPidVnodePathInfo = 9 // PROC_PIDVNODEPATHINFO: proc_vnodepathinfo
	procPidFDSocketInfo  = 3 // PROC_PIDFDSOCKETINFO: socket_fdinfo
	procFDInfoSize       = 8 // proc_fdinfo: int32 fd, uint32 type
	proxFDTypeSocket     = 2 // PROX_FDTYPE_SOCKET
	procPidPathMaxSize   = 4096
	socketFDInfoSize     = 792
	vnodePathInfoSize    = 2352
	vnodeCwdPathOffset   = 152 // proc_vnodepathinfo.pvi_cdir.vip_path
	vnodePathMax         = 1024

	// Offsets within socket_fdinfo.
	soiSoOffset      = 160 // psi.soi_so: the kernel's ID for the socket
	soiKindOffset    = 256 // psi.soi_kind
	tcpLPortOffset   = 268 // psi.soi_proto.pri_tcp.tcpsi_ini.insi_lport
	tcpStateOffset   = 344 // psi.soi_proto.pri_tcp.tcpsi_state
	sockInfoKindTCP  = 2   // SOCKINFO_TCP
	tcpSockInfoListn = 1   // TSI_S_LISTEN
)

// darwinSocket is one listening TCP socket. ID is the kernel's (obfuscated)
// address of the socket, the same value in a process's fd table and in the
// system-wide socket list, so it ties the two together like an inode does on
// Linux.
type darwinSocket struct {
	ID   uint64
	Port int
	UID  int // owner; only known for sockets from the system-wide list
}

// parseFDList returns the socket descriptors in a PROC_PIDLISTFDS buffer.
func parseFDList(buf []byte) []int32 {
	var fds []int32
	for ; len(buf) >= procFDInfoSize; buf = buf[procFDInfoSize:] {
		if binary.LittleEndian.Uint32(buf[4:8]) == proxFDTypeSocket {
			fds = append(fds, int32(binary.LittleEndian.Uint32(buf[0:4])))
		}
	}
	return fds
}

// parseSocketFDInfo decodes a socket_fdinfo. ok is false for anything but a
// listening TCP socket.
func parseSocketFDInfo(buf []byte) (s darwinSocket, ok bool) {
	if len(buf) < tcpStateOffset+4 {
		return darwinSocket{}, false
	}
	if binary.LittleEndian.Uint32(buf[soiKindOffset:]) != sockInfoKindTCP ||
		binary.LittleEndian.Uint32(buf[tcpStateOffset:]) != tcpSockInfoListn {
		return darwinSocket{}, false
	}
	return darwinSocket{
		ID:   binary.LittleEndian.Uint64(buf[soiSoOffset:]),
		Port: portFromNetworkOrder(buf[tcpLPortOffset:]),
	}, true
}

// The net.inet.tcp.pcblist_n sysctl (what `netstat -a` reads) returns an
// xinpgen header, then for every TCP socket a run of records that each start
// with their own length and kind, then a closing xinpgen. Records are padded
// to 8 bytes.
const (
	xinpgenSize = 24

	xsoSocket = 0x001 // xsocket_n
	xsoInpcb  = 0x010 // xinpcb_n; first record of each socket
	xsoTcpcb  = 0x020 // xtcpcb_n; last record of each socket

	xinpcbLPortOffset  = 18 // xinpcb_n.inp_lport, network byte order
	xsocketSoOffset    = 8  // xsocket_n.xso_so
	xsocketUIDOffset   = 64 // xsocket_n.so_uid
	xtcpcbStateOffset  = 36 // xtcpcb_n.t_state
	tcpsListen         = 1  // TCPS_LISTEN
	pcbRecordHeaderLen = 8  // uint32 length, uint32 kind
)

// parsePCBList returns the listening sockets in a net.inet.tcp.pcblist_n
// buffer. Unlike a process's fd table, it covers every user's sockets.
func parsePCBList(buf []byte) ([]darwinSocket, error) {
	if len(buf) < xinpgenSize {
		return nil, fmt.Errorf("pcblist too short: %d bytes", len(buf))
	}
	off := roundUp8(int(binary.LittleEndian.Uint32(buf)))
	if off < xinpgenSize {
		return nil, fmt.Errorf("pcblist header claims %d bytes", off)
	}

	var out []darwinSocket
	var cur darwinSocket
	for off+pcbRecordHeaderLen <= len(buf) {
		n := int(binary.LittleEndian.Uint32(buf[off:]))
		kind := binary.LittleEndian.Uint32(buf[off+4:])
		if n <= xinpgenSize { // the closing xinpgen
			break
		}
		if n > len(buf)-off {
			return nil, fmt.Errorf("pcblist record at %d claims %d bytes but only %d remain", off, n, len(buf)-off)
		}
		rec := buf[off : off+n]
		switch kind {
		case xsoInpcb:
			if n < xinpcbLPortOffset+2 {
				return nil, fmt.Errorf("pcblist inpcb record is %d bytes", n)
			}
			cur = darwinSocket{Port: portFromNetworkOrder(rec[xinpcbLPortOffset:])}
		case xsoSocket:
			if n < xsocketUIDOffset+4 {
				return nil, fmt.Errorf("pcblist socket record is %d bytes", n)
			}
			cur.ID = binary.LittleEndian.Uint64(rec[xsocketSoOffset:])
			cur.UID = int(binary.LittleEndian.Uint32(rec[xsocketUIDOffset:]))
		case xsoTcpcb:
			if n < xtcpcbStateOffset+4 {
				return nil, fmt.Errorf("pcblist tcpcb record is %d bytes", n)
			}
			if binary.LittleEndian.Uint32(rec[xtcpcbStateOffset:]) == tcpsListen {
				out = append(out, cur)
			}
			cur = darwinSocket{}
		}
		off += roundUp8(n)
	}
	return out, nil
}

func roundUp8(n int) int { return (n + 7) &^ 7 }

// parseProcArgs turns a kern.procargs2 buffer into a readable command line.
// The buffer holds an int32 argc, the executable path, NUL padding, then the
// arguments (followed by the environment, which is ignored), all
// NUL-terminated.
func parseProcArgs(buf []byte) (string, error) {
	if len(buf) < 4 {
		return "", fmt.Errorf("procargs too short: %d bytes", len(buf))
	}
	argc := int(int32(binary.LittleEndian.Uint32(buf)))
	rest := buf[4:]
	end := bytes.IndexByte(rest, 0) // end of the executable path
	if argc <= 0 || end < 0 {
		return "", fmt.Errorf("malformed procargs (argc %d)", argc)
	}
	rest = bytes.TrimLeft(rest[end:], "\x00")

	args := rest
	n := 0
	for i := 0; i < argc; i++ {
		j := bytes.IndexByte(args[n:], 0)
		if j < 0 { // last argument cut off by the buffer
			n = len(args)
			break
		}
		n += j + 1
	}
	return ParseCmdline(args[:n]), nil
}

// cString returns the bytes of a NUL-terminated string in a fixed-size field.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// hiddenSockets returns the sockets in the system-wide list that belong to
// processes portfind couldn't inspect: those not among the sockets seen in
// any process's descriptors, and owned by another user. A socket of our own
// user that wasn't seen was opened after the descriptors were read, and will
// be in the next scan.
func hiddenSockets[V any](all []darwinSocket, seen map[uint64]V, euid int) []darwinSocket {
	var hidden []darwinSocket
	for _, s := range all {
		if _, ok := seen[s.ID]; !ok && s.UID != euid {
			hidden = append(hidden, s)
		}
	}
	return hidden
}

// socketOwner picks which of the processes sharing a listening socket to
// show. A server that forks workers shares its socket with them; the one to
// kill is the parent, i.e. the holder whose own parent isn't a holder too.
// Ties go to the lowest PID.
func socketOwner(holders []int, parentOf map[int]int) int {
	isHolder := make(map[int]bool, len(holders))
	for _, pid := range holders {
		isHolder[pid] = true
	}
	owner, ownerIsRoot := 0, false
	for _, pid := range holders {
		root := !isHolder[parentOf[pid]]
		switch {
		case owner == 0,
			root && !ownerIsRoot,
			root == ownerIsRoot && pid < owner:
			owner, ownerIsRoot = pid, root
		}
	}
	return owner
}
