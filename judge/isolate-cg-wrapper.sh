#!/bin/bash
# Wrapper script for isolate that removes cgroup-related flags
# This is needed for cgroups v2 systems (Docker Desktop/WSL2)

ARGS=()
for arg in "$@"; do
    case "$arg" in
        --cg|--cg-mem*|--cg-timing|--cg-enable*)
            # Skip cgroup-related flags
            ;;
        *)
            ARGS+=("$arg")
            ;;
    esac
done

exec /usr/local/bin/isolate.real "${ARGS[@]}"
