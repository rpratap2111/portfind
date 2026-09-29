package inspector

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// Real lines from /proc/net/tcp and /proc/net/tcp6 (Ubuntu 24.04, WSL2).
const (
	procTCPHeader  = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	procTCP6Header = "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

	resolvedLine    = "   1: 3600007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   991        0 6063 1 0000000000000000 100 0 0 10 5\n"
	dnsRootLine     = "   0: FEFFFF0A:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 7182 1 0000000000000000 100 0 0 10 0\n"
	pythonV4Line    = "   3: 0100007F:22C3 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 14515 1 0000000000000000 100 0 0 10 0\n"
	establishedLine = "   4: 0100007F:22C3 0100007F:D6D8 01 00000000:00000000 00:00000000 00000000  1000        0 14600 1 0000000000000000 20 4 30 10 -1\n"
	pythonV6Line    = "   0: 00000000000000000000000001000000:22C2 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5011 1 0000000000000000 100 0 0 10 0\n"
	anyV6Line       = "   1: 00000000000000000000000000000000:0BB8 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 5012 1 0000000000000000 100 0 0 10 0\n"
	mappedV4Line    = "   2: 0000000000000000FFFF00000100007F:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 5013 1 0000000000000000 100 0 0 10 0\n"
)

func TestParseProcNetTCP(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    []procSocket
		wantErr string
	}{
		{"header only", procTCPHeader, nil, ""},
		{"IPv4 listeners: address words are little-endian, port big-endian",
			procTCPHeader + dnsRootLine + resolvedLine + pythonV4Line,
			[]procSocket{
				{Addr: netip.MustParseAddr("10.255.255.254"), Port: 53, UID: 0, Inode: 7182},
				{Addr: netip.MustParseAddr("127.0.0.54"), Port: 53, UID: 991, Inode: 6063},
				{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8899, UID: 1000, Inode: 14515},
			}, ""},
		{"non-listening sockets (st 01 ESTABLISHED) are skipped",
			procTCPHeader + pythonV4Line + establishedLine,
			[]procSocket{{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8899, UID: 1000, Inode: 14515}}, ""},
		{"IPv6 loopback, any, and v4-mapped",
			procTCP6Header + pythonV6Line + anyV6Line + mappedV4Line,
			[]procSocket{
				{Addr: netip.MustParseAddr("::1"), Port: 8898, UID: 1000, Inode: 5011},
				{Addr: netip.MustParseAddr("::"), Port: 3000, UID: 1000, Inode: 5012},
				{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8080, UID: 0, Inode: 5013},
			}, ""},
		{"blank trailing lines are fine", procTCPHeader + pythonV4Line + "\n\n",
			[]procSocket{{Addr: netip.MustParseAddr("127.0.0.1"), Port: 8899, UID: 1000, Inode: 14515}}, ""},
		{"truncated line", procTCPHeader + "   0: 0100007F:22C3 00000000:0000 0A\n", nil, "at least 10 fields"},
		{"bad address", procTCPHeader + strings.Replace(pythonV4Line, "0100007F", "01ZZ007F", 1), nil, "bad address"},
		{"bad inode", procTCPHeader + strings.Replace(pythonV4Line, " 14515 ", " -5 ", 1), nil, "bad inode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProcNetTCP([]byte(tt.data))
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
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestSocketInode(t *testing.T) {
	tests := []struct {
		link  string
		inode uint64
		ok    bool
	}{
		{"socket:[14515]", 14515, true},
		{"socket:[0]", 0, true},
		{"pipe:[14515]", 0, false},
		{"/dev/null", 0, false},
		{"socket:[abc]", 0, false},
		{"socket:[14515", 0, false},
		{"anon_inode:[eventpoll]", 0, false},
	}
	for _, tt := range tests {
		inode, ok := socketInode(tt.link)
		if inode != tt.inode || ok != tt.ok {
			t.Errorf("socketInode(%q) = %d, %v; want %d, %v", tt.link, inode, ok, tt.inode, tt.ok)
		}
	}
}

// statLine builds a /proc/<pid>/stat line in the kernel's format.
func statLine(pid, comm, state, ppid, start string) string {
	// pid (comm) state ppid pgrp session tty tpgid flags minflt cminflt majflt
	// cmajflt utime stime cutime cstime priority nice threads itrealvalue
	// starttime vsize rss ...
	return pid + " (" + comm + ") " + state + " " + ppid +
		" 1234 1000 34816 1234 4194304 900 0 0 0 12 3 0 0 20 0 1 0 " + start + " 26763264 5120 18446744073709551615\n"
}

func TestParseProcStat(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    procStat
		wantErr bool
	}{
		{"simple", statLine("1234", "python3", "S", "1000", "887766"),
			procStat{Comm: "python3", State: 'S', PPID: 1000, StartTicks: 887766}, false},
		{"comm with spaces and parentheses", statLine("99", "tmux: server (x)", "R", "1", "5"),
			procStat{Comm: "tmux: server (x)", State: 'R', PPID: 1, StartTicks: 5}, false},
		{"zombie", statLine("77", "node", "Z", "76", "10"),
			procStat{Comm: "node", State: 'Z', PPID: 76, StartTicks: 10}, false},
		{"no parentheses", "1234 python3 S 1000", procStat{}, true},
		{"too few fields", "1234 (x) S 1 2 3\n", procStat{}, true},
		{"bad ppid", strings.Replace(statLine("5", "x", "S", "1", "9"), ") S 1 ", ") S one ", 1), procStat{}, true},
	}
	for _, tt := range tests {
		got, err := parseProcStat([]byte(tt.data))
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestParseBootTime(t *testing.T) {
	got, err := parseBootTime([]byte("cpu  210 0 366\nintr 1 2 3\nctxt 43805\nbtime 1790531191\nprocesses 772\n"))
	if err != nil || got != 1790531191 {
		t.Fatalf("parseBootTime = %d, %v", got, err)
	}
	if _, err := parseBootTime([]byte("cpu 1 2 3\n")); err == nil {
		t.Fatal("want an error when btime is missing")
	}
}

func TestParseCmdline(t *testing.T) {
	tests := map[string]string{
		"python3\x00-m\x00http.server\x008899\x00": "python3 -m http.server 8899",
		"node\x00/home/me/My App/server.js\x00":    `node "/home/me/My App/server.js"`,
		"nginx: master process\x00":                `"nginx: master process"`, // processes that rewrite argv
		"":                                         "",
	}
	for in, want := range tests {
		if got := ParseCmdline([]byte(in)); got != want {
			t.Errorf("ParseCmdline(%q) = %q, want %q", in, got, want)
		}
	}
}
