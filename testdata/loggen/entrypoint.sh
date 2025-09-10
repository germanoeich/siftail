#!/bin/sh
set -eu

# MODE: text | json | mixed
MODE="${MODE:-mixed}"
SERVICE_NAME="${SERVICE_NAME:-app}"

i=1
while true; do
  # Rotate through common levels
  idx=$(( (i - 1) % 4 ))
  case "$idx" in
    0) LVL="DEBUG" ;;
    1) LVL="INFO"  ;;
    2) LVL="WARN"  ;;
    3) LVL="ERROR" ;;
  esac

  TS=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  case "$MODE" in
    text)
      echo "$TS [$LVL] $SERVICE_NAME text log line $i from $HOSTNAME"
      ;;
    json)
      printf '{"time":"%s","level":"%s","service":"%s","msg":"json log line %d"}\n' "$TS" "$LVL" "$SERVICE_NAME" "$i"
      ;;
    mixed|*)
      if [ $((i % 2)) -eq 0 ]; then
        printf '{"time":"%s","level":"%s","service":"%s","msg":"json log line %d"}\n' "$TS" "$LVL" "$SERVICE_NAME" "$i"
      else
        echo "$TS [$LVL] $SERVICE_NAME text log line $i from $HOSTNAME"
      fi
      ;;
  esac

  sleep 1
  i=$((i + 1))
done
