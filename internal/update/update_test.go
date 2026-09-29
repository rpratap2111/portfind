package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTagFromURL(t *testing.T) {
	tests := []struct {
		loc, want string
		ok        bool
	}{
		{"https://github.com/rpratap2111/portfind/releases/tag/v1.2.0", "v1.2.0", true},
		{"https://github.com/o/r/releases/tag/v10.0.1/", "v10.0.1", true},
		{"/o/r/releases/tag/v1.2.0-rc.1", "v1.2.0-rc.1", true},
		{"https://github.com/o/r/releases", "", false},            // no releases yet
		{"https://github.com/o/r/releases/tag/latest", "", false}, // not a version
	}
	for _, tt := range tests {
		got, err := tagFromURL(tt.loc)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("tagFromURL(%q) = %q, %v; want %q ok=%v", tt.loc, got, err, tt.want, tt.ok)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.0", "1.1.1", true},
		{"v1.10.0", "1.9.0", true}, // numeric, not string, comparison
		{"v1.1.1", "1.1.1", false},
		{"v1.1.0", "1.1.1", false}, // never downgrade
		{"v1.1.1", "1.1.1-SNAPSHOT-abc123", true},
		{"v2.0.0", "v1.9.9", true}, // a "v" on current is fine too
	}
	for _, tt := range tests {
		if got := isNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
		ok                 bool
	}{
		{"windows", "amd64", "portfind_windows_amd64.zip", true},
		{"windows", "arm64", "portfind_windows_arm64.zip", true},
		{"linux", "amd64", "portfind_linux_amd64.tar.gz", true},
		{"linux", "arm64", "portfind_linux_arm64.tar.gz", true},
		{"linux", "386", "", false},
		{"darwin", "arm64", "", false},
	}
	for _, tt := range tests {
		got, err := assetName(tt.goos, tt.goarch)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("assetName(%s, %s) = %q, %v", tt.goos, tt.goarch, got, err)
		}
	}
}

func sumLine(data []byte, name string) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:]) + "  " + name + "\n"
}

func TestVerify(t *testing.T) {
	archive := []byte("archive bytes")
	sums := []byte(sumLine([]byte("other"), "portfind_linux_arm64.tar.gz") + sumLine(archive, "portfind_linux_amd64.tar.gz"))
	if err := verify(archive, sums, "portfind_linux_amd64.tar.gz"); err != nil {
		t.Errorf("valid archive rejected: %v", err)
	}
	if err := verify([]byte("tampered"), sums, "portfind_linux_amd64.tar.gz"); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("tampered archive: err = %v, want mismatch", err)
	}
	if err := verify(archive, sums, "portfind_windows_amd64.zip"); err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Errorf("missing entry: err = %v", err)
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	release := map[string]string{
		"portfind.exe": "NEW-TUI", "portfind-tray.exe": "NEW-TRAY",
		"README.md": "docs", "LICENSE": "mit", // not executables: skipped
		"../portfind-evil.exe": "x", "sub/portfind.exe": "x", // outside the top level: skipped
	}
	for _, tc := range []struct {
		asset string
		data  []byte
	}{
		{"portfind_windows_amd64.zip", makeZip(t, release)},
		{"portfind_linux_amd64.tar.gz", makeTarGz(t, release)},
	} {
		files, err := extract(tc.data, tc.asset)
		if err != nil {
			t.Fatalf("%s: %v", tc.asset, err)
		}
		if len(files) != 2 || string(files["portfind.exe"]) != "NEW-TUI" || string(files["portfind-tray.exe"]) != "NEW-TRAY" {
			t.Errorf("%s: extracted %v", tc.asset, keys(files))
		}
	}
	if _, err := extract(makeZip(t, map[string]string{"README.md": "x"}), "a.zip"); err == nil {
		t.Error("an archive without executables should be an error")
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// fakeRelease serves /releases/latest (a redirect to tag) and the assets.
func fakeRelease(t *testing.T, tag, asset string, archive, sums []byte) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	// Like GitHub, downloads redirect to a separate file host.
	mux.HandleFunc("/releases/download/"+tag+"/"+asset, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/objects/"+asset, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/objects/checksums.txt", http.StatusFound)
	})
	mux.HandleFunc("/objects/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/objects/checksums.txt", func(w http.ResponseWriter, r *http.Request) { w.Write(sums) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/releases"
}

// testUpdater sets up an install dir holding old copies of the programs.
func testUpdater(t *testing.T, releases, current string) (*Updater, string) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "portfind.exe")
	os.WriteFile(exe, []byte("OLD-TUI"), 0o755)
	os.WriteFile(filepath.Join(dir, "portfind-tray.exe"), []byte("OLD-TRAY"), 0o755)
	u, err := New(current)
	if err != nil {
		t.Fatal(err)
	}
	u.Releases, u.GOOS, u.GOARCH, u.Exe = releases, "windows", "amd64", exe
	u.checkBinary = func(bin, tag string) error { return nil }
	return u, dir
}

func read(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestRunUpdates(t *testing.T) {
	asset := "portfind_windows_amd64.zip"
	archive := makeZip(t, map[string]string{"portfind.exe": "NEW-TUI", "portfind-tray.exe": "NEW-TRAY", "README.md": "d"})
	u, dir := testUpdater(t, fakeRelease(t, "v9.9.9", asset, archive, []byte(sumLine(archive, asset))), "1.1.1")

	var checked string
	u.checkBinary = func(bin, tag string) error { checked = filepath.Base(bin) + "@" + tag; return nil }
	res, err := u.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || res.From != "v1.1.1" || res.To != "v9.9.9" {
		t.Fatalf("result = %+v", res)
	}
	if res.Files[len(res.Files)-1] != "portfind.exe" {
		t.Errorf("the running program must be replaced last, got order %v", res.Files)
	}
	if checked != "portfind.new.exe@v9.9.9" {
		t.Errorf("staged binary checked as %q", checked)
	}
	if read(t, filepath.Join(dir, "portfind.exe")) != "NEW-TUI" || read(t, filepath.Join(dir, "portfind-tray.exe")) != "NEW-TRAY" {
		t.Error("programs were not replaced")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.new*")); len(matches) != 0 {
		t.Errorf("staging files left behind: %v", matches)
	}
	if runtime.GOOS == "windows" && read(t, filepath.Join(dir, "portfind.exe.old")) != "OLD-TUI" {
		t.Error("on Windows the old exe should be moved aside to .old")
	}
}

func TestRunAlreadyLatest(t *testing.T) {
	u, dir := testUpdater(t, fakeRelease(t, "v1.1.1", "x", nil, nil), "1.1.1")
	res, err := u.Run()
	if err != nil || res.Updated {
		t.Fatalf("res=%+v err=%v; want no update", res, err)
	}
	if read(t, filepath.Join(dir, "portfind.exe")) != "OLD-TUI" {
		t.Error("files changed although already up to date")
	}
}

func TestRunChangesNothingOnFailure(t *testing.T) {
	asset := "portfind_windows_amd64.zip"
	archive := makeZip(t, map[string]string{"portfind.exe": "NEW-TUI", "portfind-tray.exe": "NEW-TRAY"})
	tests := []struct {
		name  string
		sums  []byte
		check func(bin, tag string) error
		want  string
	}{
		{"tampered download", []byte(sumLine([]byte("something else"), asset)), nil, "checksum mismatch"},
		{"new binary doesn't run", []byte(sumLine(archive, asset)), func(string, string) error { return errors.New("exec format error") }, "exec format error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, dir := testUpdater(t, fakeRelease(t, "v9.9.9", asset, archive, tt.sums), "1.1.1")
			if tt.check != nil {
				u.checkBinary = tt.check
			}
			_, err := u.Run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if read(t, filepath.Join(dir, "portfind.exe")) != "OLD-TUI" || read(t, filepath.Join(dir, "portfind-tray.exe")) != "OLD-TRAY" {
				t.Error("files changed although the update failed")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 2 {
				t.Errorf("leftover files after a failed update: %d entries", len(entries))
			}
		})
	}
}

func TestRunRefusesDevBuilds(t *testing.T) {
	u, _ := testUpdater(t, "http://unused.invalid", "dev")
	if _, err := u.Run(); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Fatalf("err = %v", err)
	}
}
