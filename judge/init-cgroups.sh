#!/bin/bash
# Patch isolate to work without cgroups on cgroups v2 systems

# Check if we're on cgroups v2
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
    echo "Detected cgroups v2, setting up isolate wrapper..."

    # Backup original isolate
    if [ ! -f /usr/local/bin/isolate.original ]; then
        mv /usr/local/bin/isolate /usr/local/bin/isolate.original
        chmod +x /usr/local/bin/isolate.original
    fi

    # Create wrapper script that removes cgroup flags and runs as root
    cat > /usr/local/bin/isolate << 'WRAPPER'
#!/bin/bash
ARGS=()
for arg in "$@"; do
    case "$arg" in
        --cg|--cg-mem|--cg-timing|--cg-mem=*|--cg-timing=*)
            # Skip cgroup-related flags
            ;;
        *)
            ARGS+=("$arg")
            ;;
    esac
done
exec /usr/local/bin/isolate.original "${ARGS[@]}"
WRAPPER
    chmod +x /usr/local/bin/isolate
    # Make isolate setuid root so it can be called by judge0 user
    chown root:root /usr/local/bin/isolate
    chmod 4755 /usr/local/bin/isolate
    chown root:root /usr/local/bin/isolate.original
    chmod 4755 /usr/local/bin/isolate.original

    echo "Isolate wrapper installed with setuid root."
else
    echo "Cgroups v1 detected, no patching needed."
fi

exec "$@"
