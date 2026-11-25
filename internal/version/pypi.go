package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pypiPackage is the PyPI distribution that ships the skillshare binary as a
// wheel, and the name the upgrade commands use.
const pypiPackage = "skillshare-cli"

// pypiJSONURL is PyPI's JSON API for the package. Tests point it at a fake.
var pypiJSONURL = "https://pypi.org/pypi/" + pypiPackage + "/json"

// FetchPyPILatestVersion returns the newest skillshare-cli on PyPI. A PyPI
// install can only upgrade to what PyPI has, and a release reaches PyPI after
// GitHub marks it latest, so these installs ask PyPI, as Homebrew installs
// ask Homebrew.
func FetchPyPILatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pypiJSONURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned %d", resp.StatusCode)
	}
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	// The document lists every release's files, so allow it room to grow.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&doc); err != nil {
		return "", fmt.Errorf("failed to parse PyPI response: %w", err)
	}
	if doc.Info.Version == "" {
		return "", fmt.Errorf("no version found on PyPI")
	}
	return doc.Info.Version, nil
}

// pypiSitePackages are where an installer puts a wheel's metadata, relative to
// the prefix whose bin (Scripts on Windows) holds the binary: POSIX venvs and
// --user, Debian's dist-packages, Fedora's lib64, Windows venvs and system
// Python, and the Windows --user scheme.
var pypiSitePackages = []string{
	"lib/python*/site-packages",
	"lib/python*/dist-packages",
	"lib64/python*/site-packages",
	"Lib/site-packages",
	"site-packages",
}

// detectPyPIMethod tells a binary installed from the skillshare-cli wheel by
// the package's own dist-info next to its prefix. That keeps a binary
// install.sh put in ~/.local/bin a direct install, although pip --user uses
// the same folder.
func detectPyPIMethod(execPath string) (InstallMethod, bool) {
	bin := filepath.Dir(execPath)
	if name := filepath.Base(bin); !strings.EqualFold(name, "bin") && !strings.EqualFold(name, "Scripts") {
		return InstallDirect, false
	}
	prefix := filepath.Dir(bin)
	if !hasPyPIDistInfo(prefix) {
		return InstallDirect, false
	}
	// uv tool install leaves a receipt in the tool's environment. pyvenv.cfg
	// cannot tell it apart: uv venv writes the same file.
	if _, err := os.Stat(filepath.Join(prefix, "uv-receipt.toml")); err == nil {
		return InstallUV, true
	}
	// uvx runs a tool from an environment two levels inside uv's cache, and uv
	// tags the cache root with CACHEDIR.TAG. A uv venv tags only itself.
	// ponytail: assumes uv's <cache>/archive-v0/<id> depth; a deeper layout
	// falls back to pip, as before.
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(prefix)), "CACHEDIR.TAG")); err == nil {
		return InstallUVX, true
	}
	// pipx records each app it installs in that app's environment, wherever
	// PIPX_HOME points.
	if _, err := os.Stat(filepath.Join(prefix, "pipx_metadata.json")); err == nil {
		return InstallPipx, true
	}
	return InstallPip, true
}

// ponytail: Glob reads [, * and ? in the prefix as pattern syntax; list the
// directories with os.ReadDir if such install paths show up.
func hasPyPIDistInfo(prefix string) bool {
	distInfo := strings.ReplaceAll(pypiPackage, "-", "_") + "-*.dist-info"
	for _, dir := range pypiSitePackages {
		if matches, _ := filepath.Glob(filepath.Join(prefix, filepath.FromSlash(dir), distInfo)); len(matches) > 0 {
			return true
		}
	}
	return false
}
