package version

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pythonTree creates files under a temporary root and returns it. A path
// ending in "/" is a directory; any other path is a file holding its content.
func pythonTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		dir := full
		if !strings.HasSuffix(path, "/") {
			dir = filepath.Dir(full)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if dir == full {
			continue
		}
		if err := os.WriteFile(full, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const distInfo = "skillshare_cli-0.31.0.dist-info/"

// cacheDirTag is what uv writes into its cache root and into each environment.
const cacheDirTag = "Signature: 8a477f597d28d172789f06886806bc55\n"

func TestDetectInstallMethodPyPI(t *testing.T) {
	tests := []struct {
		name  string
		exec  string
		files map[string]string
		want  InstallMethod
	}{
		{name: "uv tool", exec: "tools/skillshare-cli/bin/skillshare", files: map[string]string{
			"tools/skillshare-cli/bin/skillshare":                           "binary",
			"tools/skillshare-cli/pyvenv.cfg":                               "home = /usr/bin\nuv = 0.9.26\n",
			"tools/skillshare-cli/uv-receipt.toml":                          "[tool]\n",
			"tools/skillshare-cli/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallUV},
		// uv venv writes the same pyvenv.cfg and CACHEDIR.TAG as uv tool; only a
		// tool has a receipt.
		{name: "uv pip in a uv venv", exec: ".venv/bin/skillshare", files: map[string]string{
			".venv/bin/skillshare":                           "binary",
			".venv/pyvenv.cfg":                               "home = /usr/bin\nuv = 0.9.26\n",
			".venv/CACHEDIR.TAG":                             cacheDirTag,
			".venv/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPip},
		// uvx runs the tool from an environment inside uv's cache, whose root
		// carries the cache's own CACHEDIR.TAG.
		{name: "uvx from the uv cache", exec: "uv-cache/archive-v0/x1/bin/skillshare", files: map[string]string{
			"uv-cache/CACHEDIR.TAG":                                           cacheDirTag,
			"uv-cache/archive-v0/x1/bin/skillshare":                           "binary",
			"uv-cache/archive-v0/x1/pyvenv.cfg":                               "home = /usr/bin\nuv = 0.9.26\n",
			"uv-cache/archive-v0/x1/CACHEDIR.TAG":                             cacheDirTag,
			"uv-cache/archive-v0/x1/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallUVX},
		{name: "pipx", exec: "pipx/venvs/skillshare-cli/bin/skillshare", files: map[string]string{
			"pipx/venvs/skillshare-cli/bin/skillshare":                           "binary",
			"pipx/venvs/skillshare-cli/pyvenv.cfg":                               "home = /usr/bin\n",
			"pipx/venvs/skillshare-cli/pipx_metadata.json":                       "{}",
			"pipx/venvs/skillshare-cli/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPipx},
		{name: "custom home with app metadata", exec: "apps/venvs/skillshare-cli/bin/skillshare", files: map[string]string{
			"apps/venvs/skillshare-cli/bin/skillshare":                           "binary",
			"apps/venvs/skillshare-cli/pipx_metadata.json":                       "{}",
			"apps/venvs/skillshare-cli/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPipx},
		{name: "a plain venv under a pipx folder", exec: "pipx/venvs/mine/bin/skillshare", files: map[string]string{
			"pipx/venvs/mine/bin/skillshare":                           "binary",
			"pipx/venvs/mine/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "pip in a venv", exec: "venv/bin/skillshare", files: map[string]string{
			"venv/bin/skillshare":                           "binary",
			"venv/pyvenv.cfg":                               "home = /usr/bin\n",
			"venv/lib/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "pip --user", exec: ".local/bin/skillshare", files: map[string]string{
			".local/bin/skillshare":                           "binary",
			".local/lib/python3.11/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "Debian system pip", exec: "usr/local/bin/skillshare", files: map[string]string{
			"usr/local/bin/skillshare":                           "binary",
			"usr/local/lib/python3.11/dist-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "Fedora platlib", exec: "usr/bin/skillshare", files: map[string]string{
			"usr/bin/skillshare":                             "binary",
			"usr/lib64/python3.12/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "Windows venv", exec: "venv/Scripts/skillshare.exe", files: map[string]string{
			"venv/Scripts/skillshare.exe":        "binary",
			"venv/Lib/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "Windows user scheme", exec: "Python311/Scripts/skillshare.exe", files: map[string]string{
			"Python311/Scripts/skillshare.exe":    "binary",
			"Python311/site-packages/" + distInfo: "",
		}, want: InstallPip},
		{name: "install.sh in ~/.local/bin", exec: ".local/bin/skillshare", files: map[string]string{
			".local/bin/skillshare": "binary",
		}, want: InstallDirect},
		{name: "another package's dist-info only", exec: "venv/bin/skillshare", files: map[string]string{
			"venv/bin/skillshare": "binary",
			"venv/lib/python3.12/site-packages/other_tool-1.0.dist-info/": "",
		}, want: InstallDirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := pythonTree(t, tt.files)
			execPath := filepath.Join(root, filepath.FromSlash(tt.exec))
			if got := DetectInstallMethod(execPath); got != tt.want {
				t.Fatalf("DetectInstallMethod(%q) = %v, want %v", tt.exec, got, tt.want)
			}
		})
	}
}

func TestPyPIUpgradeCommands(t *testing.T) {
	commands := map[InstallMethod]string{
		InstallUV:   "uv tool upgrade skillshare-cli",
		InstallPipx: "pipx upgrade skillshare-cli",
		InstallPip:  "pip install --upgrade skillshare-cli",
		InstallUVX:  "uvx --from skillshare-cli@latest skillshare",
	}
	for method, want := range commands {
		if got := method.UpgradeCommand(); got != want {
			t.Errorf("%v.UpgradeCommand() = %q, want %q", method, got, want)
		}
		if !method.IsPyPI() {
			t.Errorf("%v.IsPyPI() = false, want true", method)
		}
	}
	for _, method := range []InstallMethod{InstallNix, InstallNixProfile, InstallUV, InstallPipx, InstallPip, InstallUVX} {
		if !method.IsPackageManaged() {
			t.Errorf("%v.IsPackageManaged() = false, want true", method)
		}
	}
	for _, method := range []InstallMethod{InstallDirect, InstallBrew} {
		if method.IsPackageManaged() || method.IsPyPI() {
			t.Errorf("%v: IsPackageManaged() = %v, IsPyPI() = %v; want false, false", method, method.IsPackageManaged(), method.IsPyPI())
		}
	}
}

func TestLatestVersionForPyPIAsksPyPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"info": {"version": "0.31.0"}, "releases": {}}`))
	}))
	defer srv.Close()
	old := pypiJSONURL
	pypiJSONURL = srv.URL
	t.Cleanup(func() { pypiJSONURL = old })

	for _, method := range []InstallMethod{InstallUV, InstallPipx, InstallPip, InstallUVX} {
		if got, err := latestVersionFor(method); err != nil || got != "0.31.0" {
			t.Errorf("latestVersionFor(%v) = %q, %v; want PyPI's 0.31.0", method, got, err)
		}
	}
}

func TestFetchPyPILatestVersionFailsWithoutAVersion(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not found":  func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"no version": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"info": {}}`)) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			old := pypiJSONURL
			pypiJSONURL = srv.URL
			t.Cleanup(func() { pypiJSONURL = old })
			if got, err := FetchPyPILatestVersion(); err == nil {
				t.Fatalf("FetchPyPILatestVersion() = %q, want an error", got)
			}
		})
	}
}
