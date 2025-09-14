#!/bin/sh
set -e

# IPFS-Cluster entrypoint script with ScyllaDB support

# Default values
CLUSTER_PATH=${CLUSTER_PATH:-/data/ipfs-cluster}
CLUSTER_LOGLEVEL=${CLUSTER_LOGLEVEL:-info}

# Ensure cluster directory exists and has correct permissions
mkdir -p "$CLUSTER_PATH"
chown -R cluster:cluster "$CLUSTER_PATH"

# Function to wait for ScyllaDB to be ready
wait_for_scylladb() {
    if [ -n "$SCYLLADB_HOSTS" ]; then
        echo "Waiting for ScyllaDB to be ready..."
        
        # Extract first host from comma-separated list
        FIRST_HOST=$(echo "$SCYLLADB_HOSTS" | cut -d',' -f1)
        SCYLLADB_PORT=${SCYLLADB_PORT:-9042}
        
        # Wait for ScyllaDB to be available
        timeout=60
        while [ $timeout -gt 0 ]; do
            if nc -z "$FIRST_HOST" "$SCYLLADB_PORT" 2>/dev/null; then
                echo "ScyllaDB is ready at $FIRST_HOST:$SCYLLADB_PORT"
                return 0
            fi
            echo "Waiting for ScyllaDB at $FIRST_HOST:$SCYLLADB_PORT... ($timeout seconds remaining)"
            sleep 2
            timeout=$((timeout - 2))
        done
        
        echo "Warning: ScyllaDB not available after 60 seconds, continuing anyway..."
    fi
}

# Function to initialize cluster configuration
init_cluster() {
    if [ ! -f "$CLUSTER_PATH/service.json" ]; then
        echo "Initializing IPFS-Cluster configuration..."
        
        # Initialize with default configuration
        su-exec cluster ipfs-cluster-service init
        
        # If a custom configuration template is provided, use it
        if [ -n "$CLUSTER_CONFIG_TEMPLATE" ] && [ -f "/etc/ipfs-cluster/configurations/$CLUSTER_CONFIG_TEMPLATE" ]; then
            echo "Using configuration template: $CLUSTER_CONFIG_TEMPLATE"
            cp "/etc/ipfs-cluster/configurations/$CLUSTER_CONFIG_TEMPLATE" "$CLUSTER_PATH/service.json"
            chown cluster:cluster "$CLUSTER_PATH/service.json"
        fi
        
        # Apply environment variable overrides to configuration
        apply_config_overrides
    else
        echo "Using existing IPFS-Cluster configuration"
    fi
}

# Function to apply configuration overrides from environment variables
apply_config_overrides() {
    CONFIG_FILE="$CLUSTER_PATH/service.json"
    
    if [ -f "$CONFIG_FILE" ]; then
        # Create a temporary file for jq operations
        TEMP_CONFIG=$(mktemp)
        cp "$CONFIG_FILE" "$TEMP_CONFIG"
        
        # ScyllaDB configuration overrides
        if [ -n "$SCYLLADB_HOSTS" ]; then
            # Convert comma-separated hosts to JSON array
            HOSTS_JSON=$(echo "$SCYLLADB_HOSTS" | sed 's/,/","/g' | sed 's/^/"/' | sed 's/$/"/')
            jq ".state.scylladb.hosts = [$HOSTS_JSON]" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_PORT" ]; then
            jq ".state.scylladb.port = $SCYLLADB_PORT" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_KEYSPACE" ]; then
            jq ".state.scylladb.keyspace = \"$SCYLLADB_KEYSPACE\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_USERNAME" ]; then
            jq ".state.scylladb.username = \"$SCYLLADB_USERNAME\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_PASSWORD" ]; then
            jq ".state.scylladb.password = \"$SCYLLADB_PASSWORD\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_CONSISTENCY" ]; then
            jq ".state.scylladb.consistency = \"$SCYLLADB_CONSISTENCY\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$SCYLLADB_NUM_CONNS" ]; then
            jq ".state.scylladb.num_conns = $SCYLLADB_NUM_CONNS" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        # Cluster configuration overrides
        if [ -n "$CLUSTER_PEERNAME" ]; then
            jq ".cluster.peername = \"$CLUSTER_PEERNAME\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$CLUSTER_SECRET" ]; then
            jq ".cluster.secret = \"$CLUSTER_SECRET\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$CLUSTER_BOOTSTRAP_PEERS" ]; then
            # Convert comma-separated peers to JSON array
            PEERS_JSON=$(echo "$CLUSTER_BOOTSTRAP_PEERS" | sed 's/,/","/g' | sed 's/^/"/' | sed 's/$/"/')
            jq ".cluster.bootstrap = [$PEERS_JSON]" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        # API configuration overrides
        if [ -n "$CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS" ]; then
            jq ".api.restapi.http_listen_multiaddress = \"$CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        if [ -n "$CLUSTER_IPFSPROXY_LISTENMULTIADDRESS" ]; then
            jq ".api.ipfsproxy.listen_multiaddress = \"$CLUSTER_IPFSPROXY_LISTENMULTIADDRESS\"" "$TEMP_CONFIG" > "$CONFIG_FILE"
            cp "$CONFIG_FILE" "$TEMP_CONFIG"
        fi
        
        # Clean up
        rm -f "$TEMP_CONFIG"
        chown cluster:cluster "$CONFIG_FILE"
        
        echo "Configuration overrides applied"
    fi
}

# Function to show configuration summary
show_config_summary() {
    echo "=== IPFS-Cluster Configuration Summary ==="
    echo "Cluster Path: $CLUSTER_PATH"
    echo "Log Level: $CLUSTER_LOGLEVEL"
    
    if [ -f "$CLUSTER_PATH/service.json" ]; then
        echo "ScyllaDB Hosts: $(jq -r '.state.scylladb.hosts[]' "$CLUSTER_PATH/service.json" 2>/dev/null | tr '\n' ',' | sed 's/,$//')"
        echo "ScyllaDB Keyspace: $(jq -r '.state.scylladb.keyspace' "$CLUSTER_PATH/service.json" 2>/dev/null)"
        echo "Cluster Peername: $(jq -r '.cluster.peername' "$CLUSTER_PATH/service.json" 2>/dev/null)"
        echo "REST API: $(jq -r '.api.restapi.http_listen_multiaddress' "$CLUSTER_PATH/service.json" 2>/dev/null)"
    fi
    echo "=========================================="
}

# Main execution
case "$1" in
    daemon)
        echo "Starting IPFS-Cluster daemon with ScyllaDB state storage..."
        
        # Wait for ScyllaDB if configured
        wait_for_scylladb
        
        # Initialize cluster configuration
        init_cluster
        
        # Show configuration summary
        show_config_summary
        
        # Start the daemon
        echo "Starting IPFS-Cluster daemon..."
        exec su-exec cluster ipfs-cluster-service daemon --loglevel="$CLUSTER_LOGLEVEL"
        ;;
        
    init)
        echo "Initializing IPFS-Cluster..."
        init_cluster
        echo "Initialization complete"
        ;;
        
    version)
        exec su-exec cluster ipfs-cluster-service version
        ;;
        
    *)
        # Pass through any other commands
        exec su-exec cluster ipfs-cluster-service "$@"
        ;;
esac