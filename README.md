# Twin Manager

[![CI](https://github.com/meirka8/twin/actions/workflows/ci.yml/badge.svg)](https://github.com/meirka8/twin/actions/workflows/ci.yml)

A minimalistic two-pane TUI file manager in Go. [DOCUMENTATION.md](DOCUMENTATION.md) describes the
keys and what every operation does; the [roadmap](Twin%20%E2%80%94%20Roadmap.md) says where Twin is
going.

## Running the Application

Since the code is split into multiple files, you need to run the package, not just `main.go`.

```bash
go run .
```

Or build it first:

```bash
go build .
./twin
```

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
golangci-lint run
```

CI runs all four on every push, and on pull requests from forks. `golangci-lint` uses the standard
linters, configured in `.golangci.yml`.

The tests run a real move across filesystems only when `TWIN_XDEV_DIR` names a directory on
another filesystem than the temp directory. CI mounts a tmpfs for it; locally, `/dev/shm` usually
works:

```bash
TWIN_XDEV_DIR=/dev/shm go test ./...
```
