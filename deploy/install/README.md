# deploy/install: Maintenant native installer

This directory contains the self-contained install script, its systemd unit template, and bats test suite. User documentation: [Native Linux Install](../../docs/install.md).

## Files

| File | Description |
|---|---|
| `install.sh` | POSIX install script (served at `https://install.maintenant.dev`, and attached to every release as `install.sh`) |
| `maintenant.service` | Reference systemd unit (also embedded as heredoc in `install.sh`; a test checks that the script writes exactly this unit with its default paths) |
| `test/setup.bash` | Bats helper: mock commands used by install.sh |
| `test/install_basic.bats` | Tests for platform detection, checksum verification, basic install flow |
| `test/install_args.bats` | Tests for flag parsing, flag→env mapping, env file merge |
| `test/offline.bats` | Tests for `--binary` and `--sha256sums` (install with no network access) |
| `test/service.bats` | Tests for the service restart, the atomic binary replacement, the summary and the unit written for custom paths |
| `test/uninstall.bats` | Tests for uninstall / purge |
| `test/pinning.bats` | Tests for version pinning and error messages |

## Running tests locally

```sh
# Install bats-core (Ubuntu/Debian)
apt install bats

# Run all bats tests from repo root
bats deploy/install/test/

# Run a specific test file
bats deploy/install/test/install_basic.bats
```

The CI also runs ShellCheck on the script (`.github/workflows/shellcheck.yml`), and `go test ./internal/app/...` checks the script against the flag registry in `internal/app/flags.go`: `BOOL_FLAGS`, `VALUE_FLAGS`, the `--help` text and the flag→env mapping must list exactly the registry's configuration flags. Adding a flag to the binary therefore means adding it to the script too.

## Dev mode

The script always needs root, and it creates the `maintenant` system user (and adds it to the `docker` group when that group exists). Moving the paths keeps the files away from `/usr/local/bin`, `/etc` and `/var/lib`, but not the user: run it in a throwaway VM or container.

```sh
mkdir -p /tmp/maintenant-test
sudo MAINTENANT_INSTALL_DIR=/tmp/maintenant-test \
     MAINTENANT_DATA_DIR=/tmp/maintenant-data-test \
     MAINTENANT_CONFIG_DIR=/tmp/maintenant-config-test \
     bash deploy/install/install.sh --no-service --skip-cosign
```

The install directory must exist before the run: the script writes the binary next to its destination and renames it, and does not create that directory.

## Version pinning

```sh
# Install a specific version: set the variable on the bash side of the pipe
curl -fsSL https://install.maintenant.dev | sudo MAINTENANT_VERSION=v1.8.0 bash
```

`MAINTENANT_VERSION=v1.8.0 curl … | sudo bash` does not pin anything: the assignment only reaches `curl`.

Note: version pinning only works for releases ≥ v1.6.0 (the first release containing standalone binaries).

## Script versioning

The script carries a `SCRIPT_VERSION=__GIT_SHA__` placeholder. The release workflow (`.github/workflows/release.yml`) replaces it with the release tag when it renders the `install.sh` asset, and the script stamps that value into the header of `/etc/maintenant/maintenant.env`. This allows bug reports to identify exactly which script version was executed.
