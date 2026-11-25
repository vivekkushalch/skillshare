# Feature Proposal: Install the CLI from PyPI with uv, pipx or pip

Issue: [#236](https://github.com/runkids/skillshare/issues/236)

## Problem

Many people who use AI coding tools already have `uv` or `pipx`, but not Homebrew or Nix, and on Windows `install.ps1` is the only route. They cannot run `uv tool install skillshare` today. A `uvx` one-off run (`uvx --from skillshare-cli skillshare doctor`) is also impossible, though it suits trying skillshare without installing it.

PyPI also gives an install path that is the same on macOS, Linux and Windows, and that upgrades through the tool the user already knows.

## Proposed Solution

Publish the release binaries to PyPI as platform wheels, the way `ruff` and `uv` ship their Rust binaries. No Python code runs: each wheel contains only the Go binary, installed as the `skillshare` script.

### Packaging

- The PyPI name is `skillshare-cli`, because `skillshare` on PyPI is an unrelated course-download library.
- `scripts/release/release.py` builds six wheels from the six GoReleaser archives, during the `prepare-assets` step that already builds the Homebrew formula. The wheel binary is the archive's binary, byte for byte; nothing is rebuilt.
- The tags are `macosx_12_0_{x86_64,arm64}`, `manylinux_2_17` + `musllinux_1_1` for `{x86_64,aarch64}` (the binary is static, so one wheel serves glibc and Alpine), and `win_{amd64,arm64}`. Go 1.26 needs macOS 12, hence the macOS tag.
- Wheels are reproducible: fixed timestamps and a fixed file order make a rebuild byte-identical.
- `verify-assets` gains `verify_wheels`. It checks that each wheel holds exactly its archive's binary, the release version and a RECORD that matches its contents. The wheels are added to `checksums.txt` like the formula.

### Publishing

- **Build Release Draft** attaches the wheels to the draft release.
- **Publish Release** passes the exact wheels that `verify-assets` checked to a new `pypi` job, which uploads them with PyPI Trusted Publishing (`pypa/gh-action-pypi-publish`). No API token is stored. `skip-existing` keeps a rerun of Publish Release safe, matching how the workflow already tolerates reruns.
- Nothing reaches PyPI before the maintainer runs Publish Release, the same gate as Homebrew and Docker.

### The CLI after a PyPI install

A wheel's binary belongs to pip, uv or pipx. If `skillshare upgrade` replaced it in place, their records would still name the old version, and their next upgrade or uninstall would go wrong. This is the same problem Nix installs already have, so the change follows the Nix pattern:

- `internal/version` recognises a PyPI install from the `skillshare_cli-*.dist-info` folder next to the binary's prefix. It tells uv (its `pyvenv.cfg` records `uv = ...`), pipx (`<PIPX_HOME>/venvs/...`) and pip apart. A binary that `install.sh` placed in `~/.local/bin` has no dist-info, so it stays a direct install.
- `UpgradeCommand()` returns `uv tool upgrade skillshare-cli`, `pipx upgrade skillshare-cli` or `pip install --upgrade skillshare-cli`. `doctor` and the update notice already print that command, so they need no change.
- A new `IsPackageManaged()` (Nix or PyPI) replaces `IsNix()` at the places that must not replace the binary. Nix behaviour is unchanged.
- `skillshare upgrade` prints the manager's command instead of replacing the binary, as it does for Nix.
- The dashboard's `/api/version` adds `pypiUpgradeCommand`, and `/api/upgrade` returns `409 upgrade.pypi`. The update dialog explains the command in every dashboard language, next to the existing Nix note.

### Documentation

README and its translations get a "CLI: PyPI" section after "CLI: Nix". The website gets the install guide and the `upgrade` page, and `wiki/release.md` gets the PyPI steps and the one-time setup below. `ai_docs/tests/` gets a manual install, upgrade and uninstall runbook for Linux, Alpine and Windows.

### One-time maintainer setup

1. Create the `skillshare-cli` project on PyPI with a pending Trusted Publisher: owner `runkids`, repository `skillshare`, workflow `publish-release.yml`, environment `pypi`.
2. Create the `pypi` environment in the repository settings. Optionally require approval there.

No new runtime dependency is added. The release job uses the Python already on the runner and only the standard library.

## Alternatives Considered

- **A Python wrapper package that downloads the binary on first run.** It needs network access at run time, has a second update path, and breaks offline and in locked-down CI. Wheels that carry the binary avoid all three.
- **Building the wheels with maturin or a `setup.py`.** Both add a build toolchain to release for what is a zip with three metadata files. `release.py` already verifies archives, so writing the wheels there keeps one source of truth, and the result can be checked against the archive.
- **Running the manager's upgrade command from `skillshare upgrade`.** It would guess at the user's environment (which pip, which venv, `--user` or not). Printing the exact command, as Nix does, is predictable.
- **Recognising the Windows copy.** On Windows, uv and pipx copy `skillshare.exe` into `~/.local/bin` instead of linking it, so the running file has no dist-info beside it. Matching it would mean comparing it with every tool environment's binary on each version check. Without that, such an install is treated as direct: `skillshare upgrade` replaces the copy, the command keeps working, and only uv's own record of the version goes stale until `uv tool upgrade`. I left this out to keep the version check cheap.

## Scope

- [ ] Small (1-3 files, < 200 lines)
- [ ] Medium (3-10 files, 200-500 lines)
- [x] Large (10+ files, 500+ lines)

28 files and about 730 added lines, before the documentation. About 420 of those lines are tests and 11 are one-line dashboard translations, which leaves about 300 lines of code. The release script, the workflows, the Go detection and upgrade handling, and the dashboard each take a separate, reviewable commit.

A working reference implementation, with CI passing on Linux, macOS and Windows: [vivekkushalch/skillshare `feat/uv-pypi-install-release`](https://github.com/vivekkushalch/skillshare/tree/feat/uv-pypi-install-release).

The wheels built from the published `v0.31.0` archives are on TestPyPI: <https://test.pypi.org/project/skillshare-cli/>. From there, on Windows, `uvx`, `uv tool install` and `pipx install` each run `skillshare v0.31.0`, and both uninstalls leave nothing behind.

## Open Questions

1. **Project size on PyPI.** PyPI's default limit is 10 GB per project. One release adds about 63 MB of wheels, and the last three months had 81 releases, so the limit would be reached in roughly six months. Options: ask PyPI for a higher limit when the project is created (a routine request for binary packages), delete old releases from PyPI on a schedule, or publish to PyPI only for minor releases. Which fits the release cadence?
2. **Name.** Is `skillshare-cli` acceptable, or would you rather ask PyPI to transfer `skillshare`, or use another name?
3. **Which installers to document.** `uv tool install` and `pipx install` are the reliable paths. `pip install` works too, but into whatever environment is active. Should the README mention pip, or only uv and pipx?
4. **The Windows copy** described under Alternatives Considered: is treating it as a direct install acceptable?
