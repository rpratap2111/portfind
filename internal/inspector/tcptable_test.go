package inspector

import (
	"encoding/binary"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// tcp4Row builds one MIB_TCPROW_OWNER_PID as Windows lays it out: DWORDs in
// little-endian, except the address (network order) and the port (network
// order in the low two bytes).
func tcp4Row(addr string, port uint16, pid uint32) []byte {
	row := make([]byte, tcp4RowSize)
	binary.LittleEndian.PutUint32(row[0:], 2) // dwState: MIB_TCP_STATE_LISTEN
	a := netip.MustParseAddr(addr).As4()
	copy(row[4:8], a[:])
	binary.BigEndian.PutUint16(row[8:10], port)
	binary.LittleEndian.PutUint32(row[20:24], pid)
	return row
}

// tcp6Row builds one MIB_TCP6ROW_OWNER_PID.
func tcp6Row(addr string, port uint16, pid uint32) []byte {
	row := make([]byte, tcp6RowSize)
	a := netip.MustParseAddr(addr).As16()
	copy(row[0:16], a[:])
	binary.LittleEndian.PutUint32(row[16:], 0) // scope ID
	binary.BigEndian.PutUint16(row[20:22], port)
	binary.LittleEndian.PutUint32(row[48:], 2) // state
	binary.LittleEndian.PutUint32(row[52:56], pid)
	return row
}

// table prefixes rows with a DWORD row count.
func table(count uint32, rows ...[]byte) []byte {
	buf := binary.LittleEndian.AppendUint32(nil, count)
	for _, r := range rows {
		buf = append(buf, r...)
	}
	return buf
}

func l(addr string, port, pid int) listener {
	return listener{Addr: netip.MustParseAddr(addr), Port: port, PID: pid}
}

func TestParseTCP4Table(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    []listener
		wantErr string
	}{
		{"empty table", table(0), []listener{}, ""},
		{"one row", table(1, tcp4Row("127.0.0.1", 3000, 1234)), []listener{l("127.0.0.1", 3000, 1234)}, ""},
		{"port is big-endian (8080 = 0x1F90)", table(1, tcp4Row("0.0.0.0", 8080, 7)), []listener{l("0.0.0.0", 8080, 7)}, ""},
		{"extreme ports and PIDs", table(2, tcp4Row("10.0.0.1", 1, 4), tcp4Row("10.0.0.2", 65535, 4294967295)),
			[]listener{l("10.0.0.1", 1, 4), l("10.0.0.2", 65535, 4294967295)}, ""},
		{"trailing slack after the rows is ignored", append(table(1, tcp4Row("0.0.0.0", 80, 9)), make([]byte, 64)...),
			[]listener{l("0.0.0.0", 80, 9)}, ""},
		{"buffer shorter than the header", []byte{1, 0}, nil, "too short"},
		{"count claims more rows than present", table(2, tcp4Row("0.0.0.0", 80, 9)), nil, "claims 2 rows"},
		{"absurd count doesn't overflow", table(0xFFFFFFFF, tcp4Row("0.0.0.0", 80, 9)), nil, "claims"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTCP4Table(tt.buf)
			checkParse(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func TestParseTCP6Table(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    []listener
		wantErr string
	}{
		{"loopback and any", table(2, tcp6Row("::1", 5173, 42), tcp6Row("::", 443, 43)),
			[]listener{l("::1", 5173, 42), l("::", 443, 43)}, ""},
		{"full address", table(1, tcp6Row("fe80::1:2:3:4", 9229, 100)), []listener{l("fe80::1:2:3:4", 9229, 100)}, ""},
		{"a v4-sized row is too short for v6", table(1, tcp4Row("127.0.0.1", 80, 1)), nil, "claims 1 rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTCP6Table(tt.buf)
			checkParse(t, got, err, tt.want, tt.wantErr)
		})
	}
}

func checkParse(t *testing.T, got []listener, err error, want []listener, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("err = %v, want it to mention %q", err, wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestDedupeListeners(t *testing.T) {
	tests := []struct {
		name string
		in   []listener
		want []listener
	}{
		{"nothing", nil, []listener{}},
		{"same port and PID on v4 and v6 collapse, first kept",
			[]listener{l("0.0.0.0", 3000, 1), l("::", 3000, 1)},
			[]listener{l("0.0.0.0", 3000, 1)}},
		{"same port, different PIDs stay separate",
			[]listener{l("0.0.0.0", 80, 2), l("::", 80, 1)},
			[]listener{l("::", 80, 1), l("0.0.0.0", 80, 2)}},
		{"sorted by port, then PID",
			[]listener{l("::1", 8080, 5), l("0.0.0.0", 22, 9), l("127.0.0.1", 8080, 3)},
			[]listener{l("0.0.0.0", 22, 9), l("127.0.0.1", 8080, 3), l("::1", 8080, 5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dedupeListeners(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestHasListener(t *testing.T) {
	ls := []listener{l("0.0.0.0", 3000, 1), l("::", 8080, 2)}
	tests := []struct {
		port, pid int
		want      bool
	}{
		{3000, 1, true},
		{8080, 2, true},
		{3000, 2, false}, // right port, wrong PID: a recycled or different process
		{9999, 1, false},
	}
	for _, tt := range tests {
		if got := hasListener(ls, tt.port, tt.pid); got != tt.want {
			t.Errorf("hasListener(%d, %d) = %v, want %v", tt.port, tt.pid, got, tt.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := map[string]string{
		`C:\Program Files\nodejs\node.exe`: "node",
		`C:/tools/Python310/python.EXE`:    "python",
		"redis-server.exe":                 "redis-server",
		"ollama app.exe":                   "ollama app",
		"postgres":                         "postgres", // no extension
		".exe":                             ".exe",     // nothing left to strip to
		`C:\weird\my.exe.tool`:             "my.exe.tool",
		"":                                 "",
	}
	for in, want := range tests {
		if got := DisplayName(in); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
