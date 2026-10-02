<p align="center">
  <img src="docs/kportal-logo-dark.svg" alt="kportal logo" width="400">
</p>

<p align="center">
  <a href="https://github.com/lukaszraczylo/kportal/releases"><img src="https://img.shields.io/github/v/release/lukaszraczylo/kportal" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/lukaszraczylo/kportal" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/lukaszraczylo/kportal"><img src="https://goreportcard.com/badge/github.com/lukaszraczylo/kportal" alt="Go Report Card"></a>
</p>

# kportal

kportal runs many Kubernetes port-forwards from one YAML file and shows their status in a terminal UI. It reconnects dropped forwards, reloads the file when it changes, and can run without the UI.

![kportal Screenshot](docs/kportal-screenshot.png)

Website: <https://lukaszraczylo.github.io/kportal>

## Features

| Area | What kportal does |
|------|-------------------|
| Forwards | Forwards to `service/<name>`, `pod/<name>` (exact name or name prefix), or `pod` with a label `selector`. Only TCP is supported. |
| Terminal UI | Add (`n`), edit (`e`), delete (`d`) and toggle (`Space`) forwards without a restart. The add wizard writes the config file. |
| Table columns | `tui.columns` sets which columns the forwards table shows, their order and their width. It applies to the UI and to the `-verbose` table. |
| Add wizard | If the cluster does not allow listing namespaces, the wizard asks you to type the namespace. |
| Reconnect | Retries with exponential backoff: 1s, 2s, 4s, 8s, then 10s, with 10% jitter. It does not stop retrying. |
| Pod restarts | Forwards by pod prefix or selector connect to a new pod after a restart. |
| Health checks | `tcp-dial` or `data-transfer` check every 3s by default. A connection is stale when it is older than `maxConnectionAge` and idle, or idle longer than `maxIdleTime`. |
| Hot reload | The file watcher and `SIGHUP` reload the config. An invalid config is rejected and the previous one stays active. |
| Contexts | Several Kubernetes contexts and namespaces in one file. |
| Port conflicts | Startup and reload checks report a busy local port and the process that holds it. |
| mDNS | With `mdns.enabled: true`, each forward is reachable as `<alias>.local`. Without an alias, the resource name is used. |
| HTTP log | Per-forward request and response log in the UI, with detail view, filter, search, JSON formatting, gzip/deflate decoding and clipboard copy. Header values such as `Authorization` and `Cookie` are redacted. |
| Benchmark | Press `b` to send HTTP requests through a forward. Reports success and failure counts, min/max/avg latency, P50/P95/P99, requests per second and status codes. |
| Generate | `kportal generate` lists services in a cluster and appends the ones you pick to the config with consecutive local ports. |
| Headless | `-headless` runs without the UI and logs to stderr. `-log-format json` switches the log format. |
| Import | `--convert` turns a kftray JSON file into a kportal YAML file. |
| Shell completion | `kportal completion` prints a bash, zsh or fish completion script, or installs it. |
| Updates | `-update` checks for a newer release. `-version` prints the version. |

## Install

Homebrew (macOS):

```bash
brew install --cask lukaszraczylo/taps/kportal
```

If you installed the formula (`brew install lukaszraczylo/taps/kportal`) earlier, run `brew uninstall kportal` first.

Install script:

```bash
curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash
```

The script downloads `kportal-<version>-checksums.txt` from the same release and verifies the archive SHA-256. If [`cosign`](https://github.com/sigstore/cosign) is on `PATH`, it also verifies the keyless signature of the checksums file.

| Variable | Effect |
|----------|--------|
| `DRY_RUN=1` | Download and verify only. Do not install. |
| `SKIP_COSIGN=1` | Skip the cosign check. The SHA-256 check still runs. |

Manual download: the [releases page](https://github.com/lukaszraczylo/kportal/releases) has archives for Linux, macOS and Windows on amd64 and arm64.

Build from source (Go 1.26 or later, see `go.mod`):

```bash
git clone https://github.com/lukaszraczylo/kportal.git
cd kportal
make build && make install
```

### Verify a release signature

```bash
# Download the checksums file and its sigstore bundle from the release
cosign verify-blob \
  --certificate-identity-regexp "^https://github\.com/lukaszraczylo/shared-actions/\.github/workflows/go-release\.yaml@refs/heads/main$" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle "kportal-<version>-checksums.txt.sigstore.json" \
  kportal-<version>-checksums.txt
```

## Quick start

Create `.kportal.yaml`:

```yaml
contexts:
  - name: production
    namespaces:
      - name: backend
        forwards:
          - resource: service/postgres
            protocol: tcp
            port: 5432
            localPort: 5432
            alias: prod-db

          - resource: service/api
            protocol: tcp
            port: 8080
            localPort: 8080
            alias: api
            httpLog: true
```

Run:

```bash
kportal
```

## CLI

| Command or flag | Effect |
|-----------------|--------|
| `kportal` | Start the terminal UI. |
| `-c <file>` | Config file. Default `.kportal.yaml`. |
| `-v` | Verbose logging. In headless mode it sets the log level only. |
| `-headless` | No UI. Logs go to stderr. |
| `-check` | Validate the config and exit. |
| `-context <name>` | Forward only the named contexts. See [Select contexts](#select-contexts). |
| `-log-format text\|json` | Log format. Default `text`. |
| `-convert <file>` | Convert a kftray JSON file to kportal YAML. |
| `-convert-output <file>` | Output of `-convert`. Default `.kportal.yaml`. |
| `-update` | Check for updates and exit. |
| `-version` | Print the version and exit. |
| `kportal generate` | Add forwards from a cluster. See [Generate](#generate-forwards-from-a-cluster). |
| `kportal completion` | Print a shell completion script. |

Each flag also accepts the double-dash form, for example `--check`.

Headless example:

```bash
kportal -headless -v 2>kportal.log &
```

Completion:

```bash
kportal completion --shell zsh            # print the script
kportal completion --shell zsh --install  # install it
kportal completion --uninstall            # remove it
```

Without `--shell`, kportal detects the shell.

### Select contexts

By default kportal forwards every context in the file. `--context` limits the run to the contexts you name:

```bash
kportal --context development-team-a
kportal --context team-a --context team-b
kportal --context team-a,team-b
```

An unknown name is an error that lists the contexts in the file. Those are the `contexts[].name` values in `.kportal.yaml`, not every context in your kubeconfig.

Local-port conflicts are only reported between forwards that can run together. With `--context`, contexts you never select together can reuse a `localPort`:

```yaml
contexts:
  - name: development-team-a
    namespaces:
      - name: rate-service
        forwards:
          - &rate-service
            resource: pod/rpc-server
            protocol: tcp
            port: 50051
            localPort: 3004
            alias: rate-service

  - name: development-team-b
    namespaces:
      - name: rate-service
        forwards:
          - <<: *rate-service   # same local port, no conflict
```

Without `--context`, or with both contexts named, the duplicate port is an error.

### Generate forwards from a cluster

```bash
kportal generate --context=my-cluster
kportal generate --context=my-cluster --config=/path/to/.kportal.yaml
kportal generate --context=my-cluster --dry-run
```

| Flag | Description |
|------|-------------|
| `--context` | Required. Kubernetes context to scan. |
| `--config` | Config file to append to. Default `.kportal.yaml`. |
| `--dry-run` | Print the planned forwards. Do not change the config. |

Steps:

1. Namespaces: select with `space`, toggle all with `a`, filter with `/`.
2. Services: same keys. Services already in the config are locked. Non-TCP ports are skipped.
3. Port assignment: choose a first local port (default `10000`, minimum `1024`). Ports are assigned in order and skip ports in use.

Press `enter` on the last step to save (or print, with `--dry-run`), `b` to go back, `esc` to cancel.

## Keys

Forwards list:

| Key | Action |
|-----|--------|
| `Up` `Down` / `j` `k` | Move the selection |
| `PgUp` `PgDn` / `Ctrl+u` `Ctrl+d` | Move by 10 rows |
| `Space` / `Enter` | Toggle the forward |
| `n` | Add a forward |
| `e` | Edit the forward |
| `d` | Delete the forward (asks to confirm) |
| `b` | Benchmark the forward |
| `l` | Open the HTTP log |
| `q` / `Ctrl+c` | Quit |

In the add and edit wizard, `h` on the confirmation step toggles `httpLog`. Advanced `httpLog` keys you set in YAML stay unchanged.

Status values:

| Indicator | Meaning |
|-----------|---------|
| `● Active` | Connection healthy |
| `○ Starting` | First connection. Failed health checks show Starting for the first 10s. |
| `◐ Reconnecting` | Reconnecting after a failure |
| `✗ Error` | Connection failed |
| `○ Disabled` | Turned off in the UI |

## Configuration

### Forwards

```yaml
contexts:
  - name: <context-name>
    namespaces:
      - name: <namespace-name>
        forwards:
          - resource: <type>/<name>
            protocol: tcp
            port: <remote-port>
            localPort: <local-port>
            alias: <display-name>      # optional
            selector: <label-selector> # optional
            httpLog: true              # optional
```

| Field | Required | Description |
|-------|----------|-------------|
| `resource` | Yes | `service/<name>`, `pod/<name>`, or `pod` with `selector` |
| `protocol` | No | `tcp`. No other value is accepted. |
| `port` | Yes | Remote port |
| `localPort` | Yes | Local port |
| `alias` | No | Display name and mDNS hostname |
| `selector` | With bare `pod` | Label selector. Not allowed with `pod/<name>`. |
| `httpLog` | No | `true`, or a map of options (see [HTTP log](#http-log)) |

### Health checks and reliability

```yaml
healthCheck:
  interval: "3s"
  timeout: "2s"
  method: "data-transfer"  # or tcp-dial
  maxConnectionAge: "25m"
  maxIdleTime: "10m"

reliability:
  tcpKeepalive: "30s"
  dialTimeout: "30s"
  retryOnStale: true
  watchdogPeriod: "30s"
```

The values above are the defaults from `.kportal.yaml`.

- `tcp-dial` opens a TCP connection to the local port.
- `data-transfer` also tries to read data through the tunnel. It is the default.
- A connection past `maxConnectionAge` reconnects only when it is also idle, so a running transfer is not cut.

### mDNS

```yaml
mdns:
  enabled: true
```

- `alias: prod-db` publishes `prod-db.local`.
- Without an alias, `service/redis` publishes `redis.local`.
- A bare `pod` with a selector and no alias is not published.

Check registration:

```bash
dns-sd -B _kportal._tcp local       # macOS
avahi-browse -t _kportal._tcp       # Linux
```

### Table columns

```yaml
tui:
  columns:
    - name: alias
      width: 30      # optional, 1-200
    - name: resource
    - name: local
    - name: status
```

- Valid names: `context`, `namespace`, `alias`, `type`, `resource`, `remote`, `local`, `status`.
- List order is render order. Unlisted columns are hidden.
- `width` is the maximum text width, 1 to 200. Without it, the column keeps its default width.
- Without `tui`, the table keeps its default columns and order.
- The setting applies to the UI and to the `-verbose` table, and reloads with the file.

### HTTP log

Press `l` on a forward that has `httpLog` set.

```yaml
forwards:
  - resource: service/api
    port: 8080
    localPort: 8080
    httpLog:
      enabled: true
      includeHeaders: true   # sensitive values are redacted
      maxBodySize: 65536     # bytes, 0 = unlimited
      filterPath: "/api/"    # log only paths containing this text
      logFile: "api.log"     # also append entries to this file
```

List view columns: TIME, METHOD, STATUS, LATENCY, PATH.

| Key | Action |
|-----|--------|
| `Up` `Down` | Move |
| `Enter` | Open details |
| `g` / `G` | Top / bottom |
| `a` | Toggle auto-scroll |
| `f` | Cycle filter: All, Non-2xx, Errors |
| `/` | Search path or method |
| `c` | Clear filters |
| `q` | Close |

Detail view shows sorted request and response headers, bodies and timing. JSON bodies are pretty-printed, gzip and deflate bodies are decoded, and binary bodies show a placeholder.

| Key | Action |
|-----|--------|
| `Up` `Down` / `PgUp` `PgDn` | Scroll |
| `g` | Top |
| `c` | Copy response body |
| `Esc` / `q` / `Enter` | Back to the list |

With `includeHeaders: true`, kportal replaces these header values with `[REDACTED]` and keeps the name: `Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-Csrf-Token`, `Proxy-Authorization`, `X-Access-Token`, and any name containing `token`, `secret`, `password` or `apikey`. This cannot be turned off.

### Benchmark

Press `b` on a forward. Set the URL path (default `/`), HTTP method, concurrency and request count.

### Reload

kportal reloads when the config file changes. To reload by hand:

```bash
kill -HUP $(pgrep kportal)
```

`Ctrl+C` and `SIGTERM` shut down cleanly.

### Import from kftray

```bash
kportal --convert configs.json --convert-output .kportal.yaml
```

## Troubleshooting

Port already in use:

```bash
lsof -i :<port>
kill <pid>
```

Connection refused:

1. Check the pod: `kubectl get pods -n <namespace>`
2. Check the port: `kubectl describe pod <pod>`
3. Check the service endpoints: `kubectl get endpoints <service>`

Context not found: list your contexts with `kubectl config get-contexts`. Names with `@`, `.`, `:` or `/` (for example `admin@home` or an EKS ARN) are valid. If `--context` reports `unknown context "..."`, the name is missing from `contexts[].name` in the config file.

The add wizard shows a namespace prompt instead of a list: the cluster refused or returned an empty namespace list. Type the namespace name.

## Development

Requires Go 1.26 or later, kubectl and access to a cluster.

```bash
make build    # build the binary
make test     # run tests
make all      # fmt, vet, staticcheck, test
make install  # install locally
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Release history is in [CHANGELOG.md](CHANGELOG.md).

## Telemetry

On startup kportal sends one anonymous adoption ping: project name, version and timestamp. It has a 2-second timeout and cannot block startup.

To opt out, set `DO_NOT_TRACK=1`, `OSS_TELEMETRY_DISABLED=1` or `KPORTAL_DISABLE_TELEMETRY=1`. See [oss-telemetry](https://github.com/lukaszraczylo/oss-telemetry#disabling-telemetry) for the wire format and source.

## License

MIT. See [LICENSE](LICENSE).

## Credits

[Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lipgloss](https://github.com/charmbracelet/lipgloss), [client-go](https://github.com/kubernetes/client-go). [kftray](https://github.com/hcavarsan/kftray) was the inspiration.
