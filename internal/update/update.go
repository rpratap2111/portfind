// Package update replaces the running portfind with the latest GitHub
// release: it finds the newest tag, downloads the archive for this OS and
// CPU, verifies it against the release's checksums.txt, test-runs the new
// binary and only then swaps it in.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// ReleasesURL is where portfind's releases live.
const ReleasesURL = "https://github.com/rpratap2111/portfind/releases"

const maxDownload = 200 << 20 // refuse absurdly large downloads

// Updater holds everything an update depends on, so tests can point it at a
// fake release server and a temporary install directory.
type Updater struct {
	Releases string       // e.g. ReleasesURL
	Client   *http.Client // downloads; follows redirects (GitHub serves assets via one)
	probe    *http.Client // Latest; must not follow redirects
	GOOS     string
	GOARCH   string
	Exe      string // path of the running executable (symlinks resolved)
	Current  string // running version without "v", e.g. "1.1.1"; "dev" for local builds

	// checkBinary confirms a staged binary runs and reports the expected
	// version; checkRuns by default, replaced in tests.
	checkBinary func(bin, tag string) error
}

// Result describes what Run did.
type Result struct {
	From, To string   // versions with "v", e.g. "v1.1.1"
	Updated  bool     // false when already up to date
	Files    []string // executables replaced, e.g. ["portfind.exe", "portfind-tray.exe"]
}

// New returns an Updater for the running program.
func New(current string) (*Updater, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate the running portfind: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return &Updater{
		Releases: ReleasesURL,
		Client:   &http.Client{Timeout: 2 * time.Minute},
		probe: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		Exe:         exe,
		Current:     current,
		checkBinary: checkRuns,
	}, nil
}

// Run performs the update. It changes nothing on disk unless every check
// passes.
func (u *Updater) Run() (Result, error) {
	res := Result{From: withV(u.Current)}
	if u.Current == "dev" {
		return res, errors.New("this is a development build, not an installed release; reinstall with the installer from the README to get release updates")
	}
	latest, err := u.Latest()
	if err != nil {
		return res, err
	}
	res.To = latest
	if !isNewer(latest, u.Current) {
		return res, nil
	}

	asset, err := assetName(u.GOOS, u.GOARCH)
	if err != nil {
		return res, err
	}
	base := u.Releases + "/download/" + latest + "/"
	archive, err := u.get(base + asset)
	if err != nil {
		return res, err
	}
	sums, err := u.get(base + "checksums.txt")
	if err != nil {
		return res, err
	}
	if err := verify(archive, sums, asset); err != nil {
		return res, err
	}
	files, err := extract(archive, asset)
	if err != nil {
		return res, err
	}

	dir := filepath.Dir(u.Exe)
	cleanupOld(dir)
	staged, err := stage(dir, files)
	defer func() {
		for _, s := range staged {
			os.Remove(s) // leftovers only exist if something failed
		}
	}()
	if err != nil {
		return res, err
	}
	mainName := filepath.Base(u.Exe)
	if err := u.checkBinary(staged[mainName], latest); err != nil {
		return res, err
	}
	// The running program is swapped last, so a failure midway leaves the
	// command you just ran intact.
	for _, name := range installOrder(staged, mainName) {
		if err := replaceFile(staged[name], filepath.Join(dir, name)); err != nil {
			return res, fmt.Errorf("install %s: %w", name, err)
		}
		delete(staged, name)
		res.Files = append(res.Files, name)
	}
	res.Updated = true
	return res, nil
}

// Check reports the newest release tag and whether it is newer than the
// running version, without downloading anything.
func (u *Updater) Check() (latest string, newer bool, err error) {
	if u.Current == "dev" {
		return "", false, errors.New("development builds don't check for updates")
	}
	latest, err = u.Latest()
	if err != nil {
		return "", false, err
	}
	return latest, isNewer(latest, u.Current), nil
}

// Latest returns the newest release tag, e.g. "v1.2.0". It follows
// /releases/latest's redirect rather than calling the GitHub API, which is
// rate-limited for anonymous callers.
func (u *Updater) Latest() (string, error) {
	resp, err := u.probe.Get(u.Releases + "/latest")
	if err != nil {
		return "", fmt.Errorf("check for updates: %w", err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("check for updates: %s/latest answered %s without a redirect to a release", u.Releases, resp.Status)
	}
	return tagFromURL(loc)
}

func (u *Updater) get(url string) ([]byte, error) {
	resp, err := u.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if len(data) > maxDownload {
		return nil, fmt.Errorf("download %s: larger than %d MB, refusing", url, maxDownload>>20)
	}
	return data, nil
}

// tagFromURL extracts the tag from ".../releases/tag/v1.2.0".
func tagFromURL(loc string) (string, error) {
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("check for updates: unexpected release URL %q", loc)
	}
	tag := strings.Trim(loc[i+len("/tag/"):], "/")
	if !semver.IsValid(tag) {
		return "", fmt.Errorf("check for updates: latest release %q is not a version tag", tag)
	}
	return tag, nil
}

// isNewer reports whether release tag latest ("v1.2.0") is newer than the
// running version current ("1.1.1").
func isNewer(latest, current string) bool {
	return semver.Compare(latest, withV(current)) > 0
}

func withV(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// assetName is the release archive for an OS and CPU; the names come from
// .goreleaser.yaml.
func assetName(goos, goarch string) (string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("no release builds for %s/%s", goos, goarch)
	}
	switch goos {
	case "windows":
		return "portfind_windows_" + goarch + ".zip", nil
	case "linux":
		return "portfind_linux_" + goarch + ".tar.gz", nil
	}
	return "", fmt.Errorf("no release builds for %s/%s", goos, goarch)
}

// verify checks archive against its line in checksums.txt.
func verify(archive, sums []byte, asset string) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s; refusing an unverified update", asset)
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s (expected %s, got %s); nothing was changed", asset, want, got)
	}
	return nil
}

// extract returns the executables in a release archive, keyed by file name.
// Only top-level files named portfind* are taken, so nothing in the archive
// can write outside the install directory.
func extract(archive []byte, asset string) (map[string][]byte, error) {
	files := map[string][]byte{}
	take := func(name string) bool {
		base := path.Base(name)
		return name == base && strings.HasPrefix(base, "portfind") && base != "." && !strings.Contains(base, "..")
	}
	switch {
	case strings.HasSuffix(asset, ".zip"):
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", asset, err)
		}
		for _, f := range zr.File {
			if !take(f.Name) || f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("read %s from %s: %w", f.Name, asset, err)
			}
			data, err := io.ReadAll(io.LimitReader(rc, maxDownload))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read %s from %s: %w", f.Name, asset, err)
			}
			files[f.Name] = data
		}
	case strings.HasSuffix(asset, ".tar.gz"):
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", asset, err)
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", asset, err)
			}
			if h.Typeflag != tar.TypeReg || !take(h.Name) {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(tr, maxDownload))
			if err != nil {
				return nil, fmt.Errorf("read %s from %s: %w", h.Name, asset, err)
			}
			files[h.Name] = data
		}
	default:
		return nil, fmt.Errorf("unknown archive type %s", asset)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s contains no portfind executables", asset)
	}
	return files, nil
}

// stage writes each new executable next to its final location under a
// temporary name. The temporary name keeps the extension (portfind.new.exe)
// so Windows can still run it for checkRuns.
func stage(dir string, files map[string][]byte) (map[string]string, error) {
	staged := map[string]string{}
	for name, data := range files {
		ext := filepath.Ext(name)
		tmp := filepath.Join(dir, strings.TrimSuffix(name, ext)+".new"+ext)
		if err := os.WriteFile(tmp, data, 0o755); err != nil {
			if errors.Is(err, os.ErrPermission) {
				return staged, fmt.Errorf("can't write to %s: %w (if portfind is installed system-wide, run the update with sudo)", dir, err)
			}
			return staged, fmt.Errorf("write %s: %w", tmp, err)
		}
		staged[name] = tmp
	}
	return staged, nil
}

// checkRuns executes the staged binary with --version and checks it reports
// the version it claims to be.
func checkRuns(bin, tag string) error {
	if bin == "" {
		return errors.New("the release doesn't contain this program")
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return fmt.Errorf("the downloaded portfind doesn't run on this machine: %w", err)
	}
	// Match without the "v": releases before v1.2.0 print "portfind 1.1.1".
	if !strings.Contains(string(out), strings.TrimPrefix(tag, "v")) {
		return fmt.Errorf("the downloaded portfind reports %q, expected %s", strings.TrimSpace(string(out)), tag)
	}
	return nil
}

// installOrder puts the running program last.
func installOrder(staged map[string]string, mainName string) []string {
	var names []string
	for name := range staged {
		if name != mainName {
			names = append(names, name)
		}
	}
	if _, ok := staged[mainName]; ok {
		names = append(names, mainName)
	}
	return names
}
