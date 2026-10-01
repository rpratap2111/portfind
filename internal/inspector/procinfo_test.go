package inspector

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

// fdInfo builds one proc_fdinfo as PROC_PIDLISTFDS returns it.
func fdInfo(fd int32, fdType uint32) []byte {
	b := make([]byte, procFDInfoSize)
	binary.LittleEndian.PutUint32(b[0:], uint32(fd))
	binary.LittleEndian.PutUint32(b[4:], fdType)
	return b
}

func TestParseFDList(t *testing.T) {
	const vnode, socket, kqueue = 1, 2, 5 // PROX_FDTYPE_*
	var buf []byte
	for _, fd := range [][]byte{fdInfo(0, vnode), fdInfo(3, socket), fdInfo(4, kqueue), fdInfo(17, socket)} {
		buf = append(buf, fd...)
	}
	if got, want := parseFDList(buf), []int32{3, 17}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseFDList = %v, want %v", got, want)
	}
	// A trailing partial entry (buffer filled mid-struct) is ignored.
	if got, want := parseFDList(append(buf, 9, 0, 0)), []int32{3, 17}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseFDList with partial entry = %v, want %v", got, want)
	}
	if got := parseFDList(nil); got != nil {
		t.Errorf("parseFDList(nil) = %v, want nil", got)
	}
}

// socketFDInfo builds a socket_fdinfo with the fields portfind reads.
func socketFDInfo(kind, state uint32, port uint16, id uint64) []byte {
	b := make([]byte, socketFDInfoSize)
	binary.LittleEndian.PutUint64(b[soiSoOffset:], id)
	binary.LittleEndian.PutUint32(b[soiKindOffset:], kind)
	binary.BigEndian.PutUint16(b[tcpLPortOffset:], port) // network order, in an int32
	binary.LittleEndian.PutUint32(b[tcpStateOffset:], state)
	return b
}

func TestParseSocketFDInfo(t *testing.T) {
	const (
		kindIn, kindUnix = 1, 3 // SOCKINFO_IN (UDP), SOCKINFO_UN
		established      = 4    // TSI_S_ESTABLISHED
	)
	tests := []struct {
		name string
		buf  []byte
		want darwinSocket
		ok   bool
	}{
		{"listening TCP socket; port is in network byte order",
			socketFDInfo(sockInfoKindTCP, tcpSockInfoListn, 8899, 0xc780_23e1_3988_5af8),
			darwinSocket{ID: 0xc780_23e1_3988_5af8, Port: 8899}, true},
		{"connected TCP socket", socketFDInfo(sockInfoKindTCP, established, 8899, 1), darwinSocket{}, false},
		{"UDP socket", socketFDInfo(kindIn, 0, 5353, 2), darwinSocket{}, false},
		{"unix socket", socketFDInfo(kindUnix, 0, 0, 3), darwinSocket{}, false},
		{"short buffer", socketFDInfo(sockInfoKindTCP, tcpSockInfoListn, 8899, 4)[:tcpStateOffset], darwinSocket{}, false},
	}
	for _, tt := range tests {
		got, ok := parseSocketFDInfo(tt.buf)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: parseSocketFDInfo = %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

// pcbRecord builds one record of a pcblist_n buffer: length, kind, then
// zeroes, padded to 8 bytes as the kernel does.
func pcbRecord(size int, kind uint32) []byte {
	b := make([]byte, roundUp8(size))
	binary.LittleEndian.PutUint32(b[0:], uint32(size))
	binary.LittleEndian.PutUint32(b[4:], kind)
	return b
}

// pcbSocket builds the six records the kernel emits for one TCP socket, with
// the record sizes of macOS 26 (204 is not a multiple of 8, so the padding
// between sockets is exercised).
func pcbSocket(port uint16, id uint64, uid, state uint32) []byte {
	const xsoRcvbuf, xsoSndbuf, xsoStats = 0x002, 0x004, 0x008
	inpcb := pcbRecord(104, xsoInpcb)
	binary.BigEndian.PutUint16(inpcb[xinpcbLPortOffset:], port)
	sock := pcbRecord(104, xsoSocket)
	binary.LittleEndian.PutUint64(sock[xsocketSoOffset:], id)
	binary.LittleEndian.PutUint32(sock[xsocketUIDOffset:], uid)
	tcpcb := pcbRecord(204, xsoTcpcb)
	binary.LittleEndian.PutUint32(tcpcb[xtcpcbStateOffset:], state)

	var out []byte
	for _, rec := range [][]byte{inpcb, sock, pcbRecord(32, xsoRcvbuf), pcbRecord(32, xsoSndbuf), pcbRecord(136, xsoStats), tcpcb} {
		out = append(out, rec...)
	}
	return out
}

// pcbList wraps sockets in the opening and closing xinpgen. The second word
// of an xinpgen is a socket count, not a record kind.
func pcbList(sockets ...[]byte) []byte {
	out := pcbRecord(xinpgenSize, uint32(len(sockets)))
	for _, s := range sockets {
		out = append(out, s...)
	}
	return append(out, pcbRecord(xinpgenSize, uint32(len(sockets)))...)
}

func TestParsePCBList(t *testing.T) {
	const established = 4 // TCPS_ESTABLISHED
	sshd := pcbSocket(22, 0xaaaa, 0, tcpsListen)
	browser := pcbSocket(51510, 0xbbbb, 501, established)
	python := pcbSocket(8899, 0xcccc, 501, tcpsListen)

	tests := []struct {
		name    string
		buf     []byte
		want    []darwinSocket
		wantErr string
	}{
		{"no sockets", pcbList(), nil, ""},
		{"listeners of every user; connected sockets are skipped",
			pcbList(sshd, browser, python),
			[]darwinSocket{{ID: 0xaaaa, Port: 22, UID: 0}, {ID: 0xcccc, Port: 8899, UID: 501}}, ""},
		{"a socket count equal to a record kind doesn't confuse the header",
			pcbList(python), []darwinSocket{{ID: 0xcccc, Port: 8899, UID: 501}}, ""},
		{"records of an unknown kind are skipped",
			pcbList(append(pcbRecord(48, 0x400), python...)),
			[]darwinSocket{{ID: 0xcccc, Port: 8899, UID: 501}}, ""},
		{"empty buffer", nil, nil, "too short"},
		{"record running past the end",
			pcbList(sshd)[:xinpgenSize+150], nil, "claims 104 bytes"},
		{"record too small for its kind",
			append(pcbRecord(xinpgenSize, 1), pcbRecord(32, xsoSocket)...), nil, "socket record is 32 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePCBList(tt.buf)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// procArgs builds a kern.procargs2 buffer.
func procArgs(argc int32, exe string, padding int, args, env []string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(argc))
	b = append(b, exe...)
	b = append(b, make([]byte, 1+padding)...)
	for _, s := range append(append([]string{}, args...), env...) {
		b = append(append(b, s...), 0)
	}
	return b
}

func TestParseProcArgs(t *testing.T) {
	env := []string{"HOME=/Users/dev", "SECRET_TOKEN=hunter2"}
	tests := []struct {
		name    string
		buf     []byte
		want    string
		wantErr string
	}{
		{"arguments only: the executable path and environment are left out",
			procArgs(3, "/opt/homebrew/bin/node", 5, []string{"node", "server.js", "--port=3000"}, env),
			"node server.js --port=3000", ""},
		{"arguments with spaces are quoted",
			procArgs(2, "/Applications/My App.app/Contents/MacOS/My App", 0, []string{"/Applications/My App.app/Contents/MacOS/My App", "--serve"}, env),
			`"/Applications/My App.app/Contents/MacOS/My App" --serve`, ""},
		{"no environment", procArgs(1, "/usr/bin/nc", 3, []string{"nc"}, nil), "nc", ""},
		{"last argument cut off by the buffer",
			procArgs(2, "/bin/sh", 1, []string{"sh", "-c"}, nil)[:4+len("/bin/sh")+2+len("sh")+1+1],
			"sh -", ""},
		{"too short", []byte{1, 0}, "", "too short"},
		{"zero argc", procArgs(0, "/bin/sh", 0, nil, env), "", "malformed"},
		{"unterminated path", append([]byte{1, 0, 0, 0}, "/bin/sh"...), "", "malformed"},
	}
	for _, tt := range tests {
		got, err := parseProcArgs(tt.buf)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: parseProcArgs = %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
}

func TestSocketOwner(t *testing.T) {
	// 900 (shell) -> 1000 (server) -> 1001, 1002 (workers); 700 is unrelated.
	parentOf := map[int]int{900: 1, 1000: 900, 1001: 1000, 1002: 1000, 700: 1, 50: 60000, 60000: 1}
	tests := []struct {
		name    string
		holders []int
		want    int
	}{
		{"sole holder", []int{1000}, 1000},
		{"server and its forked workers: the server", []int{1002, 1000, 1001}, 1000},
		{"workers only (the server closed its copy): lowest PID", []int{1002, 1001}, 1001},
		{"parent has the higher PID after PID wrap-around", []int{50, 60000}, 60000},
		{"unrelated holders: lowest PID", []int{1000, 700}, 700},
		{"holder missing from the process table", []int{4242}, 4242},
	}
	for _, tt := range tests {
		if got := socketOwner(tt.holders, parentOf); got != tt.want {
			t.Errorf("%s: socketOwner(%v) = %d, want %d", tt.name, tt.holders, got, tt.want)
		}
	}
}

func TestHiddenSockets(t *testing.T) {
	const me = 501
	seen := map[uint64][]int{0xcccc: {7355}, 0xdddd: {812}}
	all := []darwinSocket{
		{ID: 0xaaaa, Port: 22, UID: 0},     // root's, in a process we can't inspect
		{ID: 0xbbbb, Port: 5432, UID: 502}, // another user's
		{ID: 0xcccc, Port: 8899, UID: me},  // ours, and seen
		{ID: 0xdddd, Port: 80, UID: 0},     // created by root but held by a process we could inspect
		{ID: 0xeeee, Port: 3000, UID: me},  // ours, opened after the descriptor walk
	}
	want := []darwinSocket{{ID: 0xaaaa, Port: 22, UID: 0}, {ID: 0xbbbb, Port: 5432, UID: 502}}
	if got := hiddenSockets(all, seen, me); !reflect.DeepEqual(got, want) {
		t.Errorf("hiddenSockets = %+v, want %+v", got, want)
	}
	if got := hiddenSockets(nil, seen, me); got != nil {
		t.Errorf("hiddenSockets(nil) = %+v, want nil", got)
	}
}
