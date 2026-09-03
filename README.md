#### Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Configuration](#configuration)
- [Commands](#commands)
- [GitHub Actions](#github-actions)
- [Full Example](#full-example)

# Overview

`cb-cicd` is a lightweight CI/CD tool for syncing ClearBlade platform resources from a local repository to a target system.

`cb-cicd`:

1. Reads a `cicd-config.json` whitelist that declares exactly which resources are managed by the pipeline.
2. Accepts a list of changed files and automatically determines which whitelisted resources need to be synced.
3. Packages only the matched resources into a zip and uploads them to the ClearBlade platform in a single atomic push.
4. Supports a `test` (dry-run) mode that shows what would change without committing anything.
5. Ships as a GitHub Actions composite action for drop-in use in existing workflows.

# Installation

#### Binary installation

Go to [cb-cicd releases](https://github.com/clearblade/cb-cicd/releases) and download the latest release for your platform.

- Unpack the archive to a location of your choice.
- Add the path of your unpacked archive to `$PATH`.

#### Source installation

Go should be installed. If it's not, install it from: [golang.org](https://golang.org/doc/install)

After Go is installed, run:

```
go install github.com/clearblade/cb-cicd@latest
```

# Configuration

`cb-cicd` requires a `cicd-config.json` file that declares which resources the pipeline is allowed to sync. By default it looks for `./cicd-config.json` in the current directory.

```json
{
  "sync_resources": [
    { "name": "MyService",    "type": "service" },
    { "name": "MyLibrary",    "type": "library" },
    { "name": "MyCollection", "type": "collection" },
    { "name": "MyTimer",      "type": "timer" },
    { "name": "MyTrigger",    "type": "trigger" }
  ]
}
```

Each entry requires a `name` and a `type`. For collection resources you can also specify row-sync options:

```json
{
  "sync_resources": [
    {
      "name": "MyCollection",
      "type": "collection",
      "push_rows": true,
      "upsert_key": "item_id",
      "selected_rows": ["row-id-1", "row-id-2"]
    }
  ]
}
```

### Supported resource types

| Type                | Description                              |
|---------------------|------------------------------------------|
| `service`           | Code service                             |
| `library`           | Code library                             |
| `collection`        | Data collection (metadata + rows)        |
| `collection_schema` | Data collection schema only              |
| `trigger`           | Trigger                                  |
| `timer`             | Timer                                    |
| `webhook`           | Webhook                                  |
| `deployment`        | Deployment                               |
| `role`              | Role                                     |
| `user`              | User record + role assignments (`users/<email>.json` and `users/roles/<email>.json`, both required) |
| `secret`            | User secret                              |
| `edge`              | Edge                                     |
| `device`            | Device                                   |
| `plugin`            | Plugin                                   |
| `portal`            | Portal                                   |
| `adaptor`           | Adapter                                  |
| `service_cache`     | Shared cache                             |
| `external_database` | External database                        |
| `bucket_set`        | Bucket set (metadata only)               |
| `bucket_set_files`  | Bucket set files                         |
| `file_store`        | File store (metadata only)               |
| `device_schema`     | Device table schema (name field unused)  |
| `user_schema`       | User table schema (name field unused)    |
| `edge_schema`       | Edge table schema (name field unused)    |

# Commands

## reconcile

**cb-cicd reconcile**: Push the ENTIRE whitelist, converging the system on the repo.

### Synopsis

```
cb-cicd reconcile
    [-email      <string>]
    [-password   <string>]
    [-system-key <string>]
    [-url        <string>]
    [-config     <path>]
    [-dry-run]
```

### Description

Packages every whitelisted resource into a zip and uploads it. Desired state is the whitelist plus the repo's content; the platform upsert converges the system onto it. There is no diff scope and no memory of previous runs, so a failed run is repaired by the next one, a whitelist-only change deploys the resource it names, and a brand-new system bootstraps with no special case. Pushing an unchanged resource is a server-side no-op, so reconciling repeatedly is safe.

`-dry-run` returns the platform's semantic diff — only resources whose meaningful content differs are reported (platform-assigned fields such as `system_key`, `uuid`, `version`, and `code_hash` are ignored and rewritten on apply) — which makes it an accurate drift report.

Reconcile never deletes anything: removing a whitelist entry stops managing a resource but leaves it on the platform. Use `prune` for that, deliberately.

## prune

**cb-cicd prune**: Delete named resources from the platform after their whitelist entries are removed.

### Synopsis

```
cb-cicd prune <type:name> [<type:name> ...]
    [-email      <string>]
    [-password   <string>]
    [-system-key <string>]
    [-url        <string>]
    [-config     <path>]
    [-dry-run]
```

### Description

The zip upload only upserts, so a resource removed from the repo keeps running in every environment it ever reached. `prune` closes that gap — statelessly and deliberately: the operator names each resource (`service:oldService timer:oldTimer`), because a whitelist removal arrives as a reviewed diff and deleting a live resource deserves a human decision. Never run it from CI.

- Refuses anything still whitelisted in `-config` — the next reconcile would just recreate it, so the request is almost certainly a mistake.
- `-dry-run` prints the plan without authenticating.
- Types with no platform delete call (schema singletons, `bucket_set_files`, `user`/`device`/`edge`) are reported as **manual** rather than skipped silently.
- Pruning a resource that is already gone fails loudly ("not found — already gone?").

#### Examples

```
cb-cicd prune -dry-run service:autoCurveGeneration timer:autoCurveGenerationTimer
```

```
cb-cicd prune service:autoCurveGeneration timer:autoCurveGenerationTimer
```

## run

**cb-cicd run**: Sync matched resources to the ClearBlade platform.

### Synopsis

```
cb-cicd run
    [-email    <string>]
    [-password <string>]
    [-system-key <string>]
    [-url      <string>]
    [-config   <path>]
    [-all]
    [-file     <path>] (repeatable)
```

### Description

Packages matched resources into a zip and uploads them to the platform. When `-file` paths are provided, only whitelisted resources whose disk paths overlap those files are synced. Use `-all` (or omit `-file`) to sync every whitelisted resource regardless of what changed.

### Options

- **email**
  ClearBlade developer email. Can also be set via `CICD_EMAIL`.

- **password**
  ClearBlade developer password. Can also be set via `CICD_PASSWORD`.

- **system-key**
  Target ClearBlade system key. Can also be set via `CICD_SYSTEM_KEY`.

- **url**
  ClearBlade platform URL, e.g., `https://platform.clearblade.com`. Can also be set via `CICD_URL`.

- **config**
  Path to `cicd-config.json`. Defaults to `./cicd-config.json`.

- **all**
  Sync every whitelisted resource. Ignores any `-file` flags.

- **file** *(repeatable)*
  A changed file path (relative to the system root). Repeat the flag for multiple files. `cb-cicd` matches each file against the resource whitelist and syncs only what changed.

#### Examples

```
cb-cicd run -all
```

```
cb-cicd run \
  -file code/services/MyService/MyService.js \
  -file data/MyCollection.json
```

```
cb-cicd run -config ./path/to/cicd-config.json -all
```

---

## test

**cb-cicd test**: Dry run — shows what would be synced without pushing.

### Synopsis

```
cb-cicd test [same flags as run]
```

### Description

Identical to `run` but calls the platform's dry-run endpoint instead of committing changes. Prints a diff of what would be created, updated, or deleted.

#### Examples

```
cb-cicd test -all
```

```
cb-cicd test -file code/services/MyService/MyService.js
```

# GitHub Actions

`cb-cicd` ships as a GitHub Actions composite action. Add it to a workflow step using `uses: clearblade/cb-cicd@latest`.

### Action inputs

| Input     | Required | Default                | Description                                          |
|-----------|----------|------------------------|------------------------------------------------------|
| `config`  | No       | `./cicd-config.json`   | Path to `cicd-config.json`                           |
| `files`   | No       | *(empty — syncs all)*  | Space-separated `-file` flags to pass to `cb-cicd`   |
| `version` | No       | `latest`               | Release version to download, e.g. `v1.2.0`          |

### Required environment variables

The action reads credentials from environment variables. Set these as GitHub Actions secrets/variables on the target environment:

| Variable            | Description                     |
|---------------------|---------------------------------|
| `CICD_EMAIL`        | ClearBlade developer email      |
| `CICD_PASSWORD`     | ClearBlade developer password   |
| `CICD_SYSTEM_KEY`   | Target system key               |
| `CICD_URL`          | ClearBlade platform URL         |

# Full Example

The `examples/` directory contains a ready-to-use GitHub Actions workflow. The pattern below syncs two independent ClearBlade systems when a pull request is opened against the `dev`, `qa`, or `prod` branches. Each system uses its own environment secrets so the correct target system is always used.

```yaml
name: Sync ClearBlade Resources

on:
  pull_request:
    branches:
      - dev
      - qa
      - prod

jobs:
  sync-system-one:
    runs-on: ubuntu-latest
    environment: ${{ github.base_ref }}

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Get changed files in system-one
        id: changed
        run: |
          FILES=$(git diff --name-only origin/${{ github.base_ref }} HEAD -- system-one/ \
            | sed 's|^system-one/||' \
            | xargs -I{} echo "-file system-one/{}" \
            | tr '\n' ' ')
          echo "files=$FILES" >> $GITHUB_OUTPUT

      - name: Sync system-one
        uses: clearblade/cb-cicd@latest
        env:
          CICD_EMAIL:      ${{ secrets.CLEARBLADE_EMAIL }}
          CICD_PASSWORD:   ${{ secrets.CLEARBLADE_PASSWORD }}
          CICD_SYSTEM_KEY: ${{ secrets.CLEARBLADE_SYSTEM_KEY }}
          CICD_URL:        ${{ vars.CLEARBLADE_URL }}
        with:
          config: ./system-one/cicd-config.json
          files:  ${{ steps.changed.outputs.files }}
```

### How it works

1. `actions/checkout` is run with `fetch-depth: 0` so the full branch history is available for the diff.
2. `git diff --name-only` produces the list of files changed between the PR branch and the base branch, scoped to the system subdirectory.
3. The file list is formatted as `-file <path>` flags and passed to the action.
4. `cb-cicd` matches those paths against the whitelist in `cicd-config.json` and syncs only the affected resources.
5. If no files in the diff match any whitelisted resource, `cb-cicd` exits cleanly with no sync performed.

### Using environments for multi-stage promotion

Create a GitHub environment for each branch (`dev`, `qa`, `prod`) and store the corresponding system credentials there. Because the workflow uses `environment: ${{ github.base_ref }}`, the correct secrets are automatically selected for each target.
