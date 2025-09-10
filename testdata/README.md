# Test Containers for siftail

This folder contains simple containers that emit logs for manual testing.

## Quick start (docker compose)

- Build and run three short-lived containers (text, json, mixed) that each write 10 lines at ~1s intervals:

```
cd testdata
docker compose up --build
```

You’ll see output on the console; containers continuously emit logs until stopped.

While they run, you can use siftail in Docker mode:

```
# In another terminal
./siftail docker
```

## Single container

Build the image and run one container emitting mixed logs:

```
# Build image
docker build -t siftail-loggen ./testdata/loggen

# Run continuously, ~1s per line
docker run --rm \
  -e MODE=mixed \
  -e SERVICE_NAME=myapp \
  siftail-loggen
```

## Modes

- `MODE=text`  → plain text lines like:
  `2025-01-01T00:00:00Z [INFO] service text log line 3 from abc123`
- `MODE=json`  → JSON lines like:
  `{ "time": "…", "level": "WARN", "service": "json", "msg": "json log line 4" }`
- `MODE=mixed` → alternates text/JSON every line.

Each container produces 10 unique lines, one per second, then exits.
