#!/bin/sh
# Increase kernel thread/pid limits (requires privileged: true)
sysctl -w kernel.pid_max=4194304 2>/dev/null || true
sysctl -w kernel.threads-max=4194304 2>/dev/null || true
ulimit -Su unlimited 2>/dev/null || true
ulimit -Hu unlimited 2>/dev/null || true
export CGO_ENABLED=0
exec "$@"
