#!/bin/sh
# Increase kernel thread/pid limits (requires privileged: true)
sysctl -w kernel.pid_max=4194304 2>/dev/null || true
sysctl -w kernel.threads-max=4194304 2>/dev/null || true
ulimit -Su unlimited 2>/dev/null || true
ulimit -Hu unlimited 2>/dev/null || true
export CGO_ENABLED=0

# Setup cgroups v1 compatibility for isolate
# Check if we're on cgroups v2 (unified hierarchy)
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
    echo "Detected cgroups v2, setting up v1 compatibility..."
    # Create legacy cgroups v1 directories if they don't exist
    mkdir -p /sys/fs/cgroup/memory 2>/dev/null || true
    mkdir -p /sys/fs/cgroup/cpuacct 2>/dev/null || true
    mkdir -p /sys/fs/cgroup/pids 2>/dev/null || true
fi

exec "$@"
