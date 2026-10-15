# Hashi gRPC Forwarder

A **single executable**, powered by Caddy, that lets an unmodified Hashi node use an API-key-protected Sui gRPC endpoint. No separate Caddy installation, Caddyfile, Python, Docker or Go toolchain is required to run a release binary.

```text
Hashi -- plaintext HTTP/2 on loopback --> forwarder
      -- HTTP/2 + verified TLS + x-api-key --> your Sui fullnode
```

This is an unofficial convenience adapter, not an official Hashi or Caddy release. It does not run Hashi, store validator keys, sign transactions, or provide a Sui node. Obtain an endpoint API key separately.

## 1. Install one binary (Linux)

Releases: <https://github.com/EasonC13/hashi-grpc-forwarder/releases>

Choose `linux_amd64` for a typical x86-64 Linux server (`uname -m` reports `x86_64`). `linux_arm64` is an alternative forwarder build for ARM Linux; it does **not** imply Hashi itself supports ARM.

Example for v0.1.0:

```bash
mkdir -p "$HOME/hashi-grpc-forwarder"
cd "$HOME/hashi-grpc-forwarder"
BASE=https://github.com/EasonC13/hashi-grpc-forwarder/releases/download/v0.1.0
curl -fL "$BASE/hashi-grpc-forwarder_linux_amd64" -o hashi-grpc-forwarder_linux_amd64
curl -fL "$BASE/SHA256SUMS" -o SHA256SUMS
sha256sum --ignore-missing --check SHA256SUMS
chmod 755 hashi-grpc-forwarder_linux_amd64
mv hashi-grpc-forwarder_linux_amd64 hashi-grpc-forwarder
./hashi-grpc-forwarder --version
```

Checksums detect corruption; obtain them through the trusted release page. They are not independent signatures. The source and third-party license inventory are available with each release. A binary still needs normal OS facilities, including DNS/network access and a system CA certificate bundle. On Debian/Ubuntu install `ca-certificates` if absent.

## 2. Configure your key

Create `.env` beside your current working directory:

```bash
umask 077
${EDITOR:-vi} .env
```

Contents (replace the placeholder privately):

```dotenv
SUI_GRPC_API_KEY=your-key-from-the-endpoint-operator
SUI_GRPC_UPSTREAM=https://sui-mainnet-grpc.giverep.com:443
FORWARDER_LISTEN=127.0.0.1:29010
```

Then:

```bash
./hashi-grpc-forwarder --check
./hashi-grpc-forwarder
# Or read an explicit dotenv file:
./hashi-grpc-forwarder --env-file /path/to/forwarder.env
```

The default `.env` is optional if you export environment variables. Existing exported values take precedence over dotenv values. `--check` validates syntax/security settings only; it does **not** authenticate to the upstream. A missing/invalid key fails closed before opening a listener. Changing the file/key requires a process restart.

Supported settings:

- `SUI_GRPC_API_KEY`: required, up to 4096 characters. Supported characters: letters, digits, `. _ ~ + / = -`. No whitespace, control characters or Caddy placeholders.
- `SUI_GRPC_UPSTREAM`: default `https://sui-mainnet-grpc.giverep.com:443`. An HTTPS origin only; no embedded credentials, path prefixes, query strings or fragments. The endpoint must support native gRPC/HTTP2.
- `FORWARDER_LISTEN`: default `127.0.0.1:29010`. Explicit IP and nonzero port; use `[::1]:29010` for IPv6 loopback.
- `FORWARDER_ALLOW_NON_LOOPBACK`: default `false`. `true` is an explicit opt-in for a protected private container network. Never expose this credential-bearing listener publicly.

## 3. Point Hashi at localhost

In your Hashi TOML configuration:

```toml
sui-rpc = "http://127.0.0.1:29010"
```

No `x-api-key` setting is needed in Hashi. The forwarder overwrites that header with its configured key on every upstream request and removes any incoming `Authorization` header. All RPC paths and streaming responses are forwarded; the key's upstream privileges determine access. This is not a Hashi-only RPC allowlist.

**Docker warning:** localhost inside a container is that container, not the host. Host-run Hashi can use the example above. For Dockerized Hashi choose one of:

- Linux host networking for Hashi and a host-run loopback forwarder, after reviewing port conflicts and loss of container network isolation.
- A forwarder in the same network namespace as Hashi (then localhost works).
- A forwarder container on a dedicated private network, listening on `0.0.0.0:29010` with `FORWARDER_ALLOW_NON_LOOPBACK=true`; use `sui-rpc = "http://sui-grpc-forwarder:29010"`. **Do not publish the forwarder port to the host or Internet.**

This first release distributes native executables, not a prebuilt container image. Do not change an existing Hashi node's networking blindly; pick the pattern matching that deployment.

## 4. Run continuously with systemd

Download/inspect the included service example, then:

```bash
sudo install -m 755 ./hashi-grpc-forwarder /usr/local/bin/hashi-grpc-forwarder
sudo install -d -m 700 /etc/hashi-grpc-forwarder
sudo install -m 600 .env /etc/hashi-grpc-forwarder/forwarder.env
curl -fL https://raw.githubusercontent.com/EasonC13/hashi-grpc-forwarder/v0.1.0/examples/hashi-grpc-forwarder.service -o hashi-grpc-forwarder.service
sudo install -m 644 hashi-grpc-forwarder.service /etc/systemd/system/hashi-grpc-forwarder.service
sudo systemctl daemon-reload
sudo systemctl enable --now hashi-grpc-forwarder
systemctl status hashi-grpc-forwarder --no-pager
journalctl -u hashi-grpc-forwarder -n 30 --no-pager
```

The `examples/` file is in the source checkout (or download it from this repository). The systemd manager reads the root-owned environment file before starting the restricted dynamic user. No root runtime privileges or writable application storage are required. This unit uses systemd EnvironmentFile syntax; keep the simple unquoted `NAME=value` settings shown here, without dotenv variable interpolation or shell commands.

To rotate a key, edit `/etc/hashi-grpc-forwarder/forwarder.env` privately and run `sudo systemctl restart hashi-grpc-forwarder`. A restart closes existing streams; Hashi must reconnect. To stop/remove:

```bash
sudo systemctl disable --now hashi-grpc-forwarder
sudo rm /etc/systemd/system/hashi-grpc-forwarder.service
sudo systemctl daemon-reload
# Remove the binary and root-only environment file separately if no longer needed.
```

## 5. Verify the real connection

If you have [`grpcurl`](https://github.com/fullstorydev/grpcurl), **do not pass a key locally**:

```bash
grpcurl -plaintext -max-time 15 -d '{}'   127.0.0.1:29010 sui.rpc.v2.LedgerService/GetServiceInfo

grpcurl -plaintext -max-time 10   -d '{"readMask":{"paths":["sequence_number"]}}'   127.0.0.1:29010 sui.rpc.v2.SubscriptionService/SubscribeCheckpoints
```

The first command should report `mainnet` for the example upstream. The second should print advancing checkpoints until your test deadline. `DeadlineExceeded` **after messages at that deadline** is expected; zero messages or immediate errors need investigation. grpcurl uses reflection here; use the appropriate proto descriptors if your own upstream disables reflection.

Troubleshooting:
- `--check` fails: inspect env names, dotenv syntax and listener/upstream rules; no key is printed in the error.
- Connection refused: confirm the forwarder is running and Hashi shares the same host/network namespace.
- `Unauthenticated` / HTTP401: invalid, revoked or wrong-operator key. Some clients instead report `server closed the stream without sending trailers` when the upstream returns an HTML401. Confirm authentication with the operator; this forwarder preserves that HTTP error rather than inventing gRPC trailers. Do not paste the key into an issue or log.
- HTTP502: check DNS, upstream availability, outbound443 and trusted CA certificates. TLS verification is always enabled; there is no insecure-skip-verify flag.
- Cannot bind: choose an unused local port. Defaults do not require privileged ports.
- Healthy local startup is not proof of upstream authentication or a fully operational Hashi node.

## Security and behavior

- Bind loopback by default. **Any local process able to reach the listener can use the upstream key's privileges.** This proxy is not a security boundary between untrusted users on one machine.
- Keep `.env`/environment files private and never commit keys, publish env dumps, or put keys in URLs/command-line arguments. The key exists in process memory/environment; privileged local administrators remain trusted.
- Caddy admin API, automatic HTTPS and config autosave are disabled. Internal Caddy request/error logging is discarded to avoid credential-bearing diagnostics; only sanitized startup/shutdown/config messages go to stdout/stderr. Use client RPC errors and your upstream's per-key monitoring for diagnosis. Do not enable debug/config dumps on a credential-bearing process.
- Upstream TLS certificate and hostname verification remain enabled, with HTTP/2 required. No application retries/replay or endpoint failover is configured, especially for transaction submissions. A failed transport can have an uncertain transaction outcome; check transaction status before resubmission.
- No intentional user rate/byte/subscription quota. Host/software capacity and transport timeouts still exist. There is no local per-key usage dashboard.
- SIGINT/SIGTERM allows a five-second shutdown grace, then closes remaining streams. Hashi owns reconnect/replay. No claim of zero-downtime failover or infinite stream duration.

The optional `scripts/smoke.py` development harness requires Python3, grpcurl and an HTTP/2-enabled curl, and reads a private `SUI_GRPC_API_KEY` from the environment. It is not needed to run the forwarder. Never put a production key in a command-line argument.

## Build and tests

Go 1.26+ is required by the pinned Caddy dependency; release builds use Go1.27.2. Dependencies are pinned in `go.mod`/`go.sum`. The HTTP/2 dependency is explicitly updated to `golang.org/x/net v0.60.0` for its security fixes.

```bash
go mod verify
go test -race -timeout 90s ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.version=dev' -o hashi-grpc-forwarder .
```

Tests use clearly labeled local TLS/HTTP2 fixtures to verify header replacement, upstream authority, error trailers, incremental streaming, cancellation and rejection of an untrusted certificate. These fixture responses are **not** live Sui data. Release acceptance additionally runs the compiled executable against a real authenticated Sui endpoint, with the key outside the repository. Linux amd64 is runtime-tested; Linux arm64 is cross-compiled, not runtime-tested. These tests do not claim a full Hashi committee/node startup or a recipient's production cutover.

## License

Wrapper source: MIT. Caddy and other dependencies retain their licenses; see [NOTICE](NOTICE) and [THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt). The binary embeds these notices: `./hashi-grpc-forwarder --licenses` works without a key or network access.
