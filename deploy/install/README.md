# deploy/install — Maintenant native installer

This directory contains the self-contained install script, its systemd unit template, and bats test suite.

## Files

| File | Description |
|---|---|
| `install.sh` | POSIX install script (served at `https://install.maintenant.dev`) |
| `maintenant.service` | Reference systemd unit (also embedded as heredoc in `install.sh`) |
| `test/setup.bash` | Bats helper: mock commands used by install.sh |
| `test/install_basic.bats` | Tests for platform detection, checksum verification, basic install flow |
| `test/install_args.bats` | Tests for flag parsing, flag→env mapping, env file merge |
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

## Dev mode (test without touching the system)

```sh
MAINTENANT_INSTALL_DIR=/tmp/maintenant-test \
MAINTENANT_DATA_DIR=/tmp/maintenant-data-test \
MAINTENANT_CONFIG_DIR=/tmp/maintenant-config-test \
bash deploy/install/install.sh --no-service --skip-cosign
```

## Pinning de version

```sh
# Install a specific version
MAINTENANT_VERSION=v1.6.0 curl -fsSL https://install.maintenant.dev | sudo -E bash

# The -E flag preserves the MAINTENANT_VERSION env var through sudo
```

Note: version pinning only works for releases ≥ v1.6.0 (the first release containing standalone binaries).

## Script versioning

The published script includes a `SCRIPT_VERSION=__GIT_SHA__` placeholder that the CI sync workflow replaces with the git short SHA. This allows bug reports to identify exactly which script version was executed.
