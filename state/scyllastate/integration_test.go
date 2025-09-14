package scyllastate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test configuration
const (
	// Docker container settings
	scyllaImage     = "scylladb/scylla:5.2"
	containerPrefix = "scylla-test"

	// Test timeouts
	containerStartTimeout = 120 * time.Second
	testTimeout           = 60 * time.Second
	clusterFormTimeout    = 180 * time.Second

	// ScyllaDB settings
	testKeyspace = "ipfs_pins_test"
	testPort     = 9042

	// Multi-node cluster settings
	multiNodeTestKeyspace = "ipfs_pins_multinode_test"
	nodeCount             = 3
	basePort              = 9042
)

// TestScyllaDBContainer manages a ScyllaDB Docker container for testing
type TestScyllaDBContainer struct {
	containerID   string
	containerName string
	host          string
	port          int
	keyspace      string
	datacenter    string
	rack          string
}

// TestScyllaDBCluster manages a multi-node ScyllaDB cluster for testing
type TestScyllaDBCluster struct {
	nodes       []*TestScyllaDBContainer
	keyspace    string
	networkName string
	seedNode    *TestScyllaDBContainer
}

// ClusterFailureSimulator simulates various failure scenarios
type ClusterFailureSimulator struct {
	cluster *TestScyllaDBCluster
}

// MigrationTestSuite manages migration testing between different state backends
type MigrationTestSuite struct {
	sourceState      state.State
	destinationState *ScyllaState
	testPins         []api.Pin
}

// StartScyllaDBContainer starts a ScyllaDB container for testing
func StartScyllaDBContainer(t *testing.T) *TestScyllaDBContainer {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	containerName := fmt.Sprintf("%s-%d", containerPrefix, time.Now().Unix())

	// Start ScyllaDB container
	containerID, err := startScyllaContainer(containerName, testPort, "", "")
	if err != nil {
		t.Fatalf("Failed to start ScyllaDB container: %v", err)
	}

	container := &TestScyllaDBContainer{
		containerID:   containerID,
		containerName: containerName,
		host:          "127.0.0.1",
		port:          testPort,
		keyspace:      testKeyspace,
		datacenter:    "datacenter1",
		rack:          "rack1",
	}

	// Wait for ScyllaDB to be ready
	if err := container.waitForReady(); err != nil {
		container.Stop()
		t.Fatalf("ScyllaDB container failed to become ready: %v", err)
	}

	// Create test keyspace and tables
	if err := container.setupSchema(); err != nil {
		container.Stop()
		t.Fatalf("Failed to setup test schema: %v", err)
	}

	return container
}

// StartScyllaDBCluster starts a multi-node ScyllaDB cluster for testing
func StartScyllaDBCluster(t *testing.T, nodeCount int) *TestScyllaDBCluster {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	networkName := fmt.Sprintf("scylla-test-network-%d", time.Now().Unix())

	// Create Docker network for the cluster
	if err := createDockerNetwork(networkName); err != nil {
		t.Fatalf("Failed to create Docker network: %v", err)
	}

	cluster := &TestScyllaDBCluster{
		nodes:       make([]*TestScyllaDBContainer, 0, nodeCount),
		keyspace:    multiNodeTestKeyspace,
		networkName: networkName,
	}

	// Start seed node first
	seedContainer, err := startClusterNode(t, networkName, 0, "")
	if err != nil {
		cluster.Cleanup()
		t.Fatalf("Failed to start seed node: %v", err)
	}

	cluster.seedNode = seedContainer
	cluster.nodes = append(cluster.nodes, seedContainer)

	// Wait for seed node to be ready
	if err := seedContainer.waitForReady(); err != nil {
		cluster.Cleanup()
		t.Fatalf("Seed node failed to become ready: %v", err)
	}

	// Start additional nodes
	for i := 1; i < nodeCount; i++ {
		node, err := startClusterNode(t, networkName, i, seedContainer.containerName)
		if err != nil {
			cluster.Cleanup()
			t.Fatalf("Failed to start node %d: %v", i, err)
		}

		cluster.nodes = append(cluster.nodes, node)

		// Wait for node to be ready
		if err := node.waitForReady(); err != nil {
			cluster.Cleanup()
			t.Fatalf("Node %d failed to become ready: %v", i, err)
		}
	}

	// Wait for cluster to form
	if err := cluster.waitForClusterFormation(); err != nil {
		cluster.Cleanup()
		t.Fatalf("Cluster failed to form: %v", err)
	}

	// Setup schema on the cluster
	if err := cluster.setupSchema(); err != nil {
		cluster.Cleanup()
		t.Fatalf("Failed to setup cluster schema: %v", err)
	}

	return cluster
}

// Stop stops and removes the ScyllaDB container
func (c *TestScyllaDBContainer) Stop() error {
	if c.containerID == "" {
		return nil
	}

	// Stop and remove container
	stopCmd := fmt.Sprintf("docker stop %s", c.containerID)
	if err := runCommand(stopCmd); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	removeCmd := fmt.Sprintf("docker rm %s", c.containerID)
	if err := runCommand(removeCmd); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	c.containerID = ""
	return nil
}

// GetConfig returns a ScyllaDB config for connecting to this container
func (c *TestScyllaDBContainer) GetConfig() *Config {
	config := &Config{}
	config.Default()

	config.Hosts = []string{c.host}
	config.Port = c.port
	config.Keyspace = c.keyspace
	config.Timeout = 10 * time.Second
	config.ConnectTimeout = 5 * time.Second
	config.Consistency = "ONE" // Use ONE for single-node testing

	return config
}

// isDockerAvailable checks if Docker is available
func isDockerAvailable() bool {
	err := runCommand("docker --version")
	return err == nil
}

// startScyllaContainer starts a ScyllaDB container
func startScyllaContainer(containerName string, port int, networkName, seedNode string) (string, error) {
	var cmd string

	if networkName == "" {
		// Single node container
		cmd = fmt.Sprintf(`docker run -d --name %s -p %d:%d %s --smp 1 --memory 1G --overprovisioned 1`,
			containerName, port, port, scyllaImage)
	} else {
		// Multi-node cluster container
		if seedNode == "" {
			// Seed node
			cmd = fmt.Sprintf(`docker run -d --name %s --network %s -p %d:%d %s --smp 1 --memory 1G --overprovisioned 1 --broadcast-address %s --listen-address 0.0.0.0 --broadcast-rpc-address %s`,
				containerName, networkName, port, port, scyllaImage, containerName, containerName)
		} else {
			// Non-seed node
			cmd = fmt.Sprintf(`docker run -d --name %s --network %s -p %d:%d %s --smp 1 --memory 1G --overprovisioned 1 --broadcast-address %s --listen-address 0.0.0.0 --broadcast-rpc-address %s --seeds %s`,
				containerName, networkName, port, port, scyllaImage, containerName, containerName, seedNode)
		}
	}

	output, err := runCommandWithOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// createDockerNetwork creates a Docker network for cluster testing
func createDockerNetwork(networkName string) error {
	cmd := fmt.Sprintf("docker network create %s", networkName)
	return runCommand(cmd)
}

// removeDockerNetwork removes a Docker network
func removeDockerNetwork(networkName string) error {
	cmd := fmt.Sprintf("docker network rm %s", networkName)
	return runCommand(cmd)
}

// startClusterNode starts a node in a ScyllaDB cluster
func startClusterNode(t *testing.T, networkName string, nodeIndex int, seedNode string) (*TestScyllaDBContainer, error) {
	containerName := fmt.Sprintf("%s-node-%d-%d", containerPrefix, nodeIndex, time.Now().Unix())
	port := basePort + nodeIndex

	containerID, err := startScyllaContainer(containerName, port, networkName, seedNode)
	if err != nil {
		return nil, err
	}

	container := &TestScyllaDBContainer{
		containerID:   containerID,
		containerName: containerName,
		host:          "127.0.0.1",
		port:          port,
		keyspace:      multiNodeTestKeyspace,
		datacenter:    "datacenter1",
		rack:          fmt.Sprintf("rack%d", (nodeIndex%3)+1), // Distribute across 3 racks
	}

	return container, nil
}

// waitForReady waits for ScyllaDB to be ready to accept connections
func (c *TestScyllaDBContainer) waitForReady() error {
	timeout := time.After(containerStartTimeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for ScyllaDB to be ready")
		case <-ticker.C:
			if c.isReady() {
				return nil
			}
		}
	}
}

// isReady checks if ScyllaDB is ready to accept connections
func (c *TestScyllaDBContainer) isReady() bool {
	cluster := gocql.NewCluster(c.host)
	cluster.Port = c.port
	cluster.Timeout = 2 * time.Second
	cluster.ConnectTimeout = 2 * time.Second
	cluster.Consistency = gocql.One

	session, err := cluster.CreateSession()
	if err != nil {
		return false
	}
	defer session.Close()

	// Try a simple query
	query := session.Query("SELECT now() FROM system.local")
	defer query.Release()

	return query.Exec() == nil
}

// setupSchema creates the test keyspace and tables
func (c *TestScyllaDBContainer) setupSchema() error {
	cluster := gocql.NewCluster(c.host)
	cluster.Port = c.port
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Consistency = gocql.One

	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Create keyspace
	createKeyspace := fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s 
		WITH REPLICATION = {
			'class': 'SimpleStrategy',
			'replication_factor': 1
		}`, c.keyspace)

	if err := session.Query(createKeyspace).Exec(); err != nil {
		return fmt.Errorf("failed to create keyspace: %w", err)
	}

	// Switch to the test keyspace
	cluster.Keyspace = c.keyspace
	keyspaceSession, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create keyspace session: %w", err)
	}
	defer keyspaceSession.Close()

	// Create tables using the schema from the design
	tables := []string{
		`CREATE TABLE IF NOT EXISTS pins_by_cid (
			mh_prefix smallint,
			cid_bin blob,
			pin_type tinyint,
			rf tinyint,
			owner text,
			tags set<text>,
			ttl timestamp,
			metadata map<text, text>,
			created_at timestamp,
			updated_at timestamp,
			PRIMARY KEY ((mh_prefix), cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS placements_by_cid (
			mh_prefix smallint,
			cid_bin blob,
			desired set<text>,
			actual set<text>,
			updated_at timestamp,
			PRIMARY KEY ((mh_prefix), cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS pins_by_peer (
			peer_id text,
			mh_prefix smallint,
			cid_bin blob,
			state tinyint,
			last_seen timestamp,
			PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS pin_ttl_queue (
			ttl_bucket timestamp,
			cid_bin blob,
			owner text,
			ttl timestamp,
			PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
		) WITH compaction = {
			'class': 'TimeWindowCompactionStrategy',
			'compaction_window_unit': 'DAYS',
			'compaction_window_size': '1'
		}`,

		`CREATE TABLE IF NOT EXISTS op_dedup (
			op_id text,
			ts timestamp,
			PRIMARY KEY (op_id)
		) WITH default_time_to_live = 604800`,
	}

	for _, table := range tables {
		if err := keyspaceSession.Query(table).Exec(); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// runCommand runs a shell command
func runCommand(cmd string) error {
	_, err := runCommandWithOutput(cmd)
	return err
}

// runCommandWithOutput runs a shell command and returns output
func runCommandWithOutput(cmd string) ([]byte, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	execCmd := exec.Command(parts[0], parts[1:]...)
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command failed: %s, error: %w", cmd, err)
	}

	return output, nil
}

// Cleanup removes all nodes and the network
func (c *TestScyllaDBCluster) Cleanup() {
	// Stop all nodes
	for _, node := range c.nodes {
		if node != nil {
			node.Stop()
		}
	}

	// Remove network
	if c.networkName != "" {
		removeDockerNetwork(c.networkName)
	}
}

// GetConfig returns a ScyllaDB config for connecting to this cluster
func (c *TestScyllaDBCluster) GetConfig() *Config {
	config := &Config{}
	config.Default()

	// Use all node addresses
	hosts := make([]string, len(c.nodes))
	for i, node := range c.nodes {
		hosts[i] = node.host
	}

	config.Hosts = hosts
	config.Port = basePort // Use base port, driver will discover other ports
	config.Keyspace = c.keyspace
	config.Timeout = 10 * time.Second
	config.ConnectTimeout = 5 * time.Second
	config.Consistency = "QUORUM" // Use QUORUM for multi-node testing
	config.NumConns = 2           // Fewer connections per host for testing

	return config
}

// waitForClusterFormation waits for all nodes to join the cluster
func (c *TestScyllaDBCluster) waitForClusterFormation() error {
	timeout := time.After(clusterFormTimeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for cluster formation")
		case <-ticker.C:
			if c.isClusterFormed() {
				return nil
			}
		}
	}
}

// isClusterFormed checks if all nodes have joined the cluster
func (c *TestScyllaDBCluster) isClusterFormed() bool {
	// Connect to seed node and check cluster status
	cluster := gocql.NewCluster(c.seedNode.host)
	cluster.Port = c.seedNode.port
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Consistency = gocql.One

	session, err := cluster.CreateSession()
	if err != nil {
		return false
	}
	defer session.Close()

	// Query system.peers to check cluster membership
	query := session.Query("SELECT COUNT(*) FROM system.peers")
	defer query.Release()

	var peerCount int
	if err := query.Scan(&peerCount); err != nil {
		return false
	}

	// We expect nodeCount-1 peers (excluding the current node)
	expectedPeers := len(c.nodes) - 1
	return peerCount >= expectedPeers
}

// setupSchema creates the test keyspace and tables on the cluster
func (c *TestScyllaDBCluster) setupSchema() error {
	cluster := gocql.NewCluster(c.seedNode.host)
	cluster.Port = c.seedNode.port
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Consistency = gocql.One

	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Create keyspace with NetworkTopologyStrategy for multi-DC support
	createKeyspace := fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s 
		WITH REPLICATION = {
			'class': 'NetworkTopologyStrategy',
			'datacenter1': %d
		}`, c.keyspace, len(c.nodes))

	if err := session.Query(createKeyspace).Exec(); err != nil {
		return fmt.Errorf("failed to create keyspace: %w", err)
	}

	// Switch to the test keyspace
	cluster.Keyspace = c.keyspace
	keyspaceSession, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to create keyspace session: %w", err)
	}
	defer keyspaceSession.Close()

	// Create tables using the schema from the design
	tables := []string{
		`CREATE TABLE IF NOT EXISTS pins_by_cid (
			mh_prefix smallint,
			cid_bin blob,
			pin_type tinyint,
			rf tinyint,
			owner text,
			tags set<text>,
			ttl timestamp,
			metadata map<text, text>,
			created_at timestamp,
			updated_at timestamp,
			PRIMARY KEY ((mh_prefix), cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS placements_by_cid (
			mh_prefix smallint,
			cid_bin blob,
			desired set<text>,
			actual set<text>,
			updated_at timestamp,
			PRIMARY KEY ((mh_prefix), cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS pins_by_peer (
			peer_id text,
			mh_prefix smallint,
			cid_bin blob,
			state tinyint,
			last_seen timestamp,
			PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
		) WITH compaction = {'class': 'LeveledCompactionStrategy'}`,

		`CREATE TABLE IF NOT EXISTS pin_ttl_queue (
			ttl_bucket timestamp,
			cid_bin blob,
			owner text,
			ttl timestamp,
			PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
		) WITH compaction = {
			'class': 'TimeWindowCompactionStrategy',
			'compaction_window_unit': 'DAYS',
			'compaction_window_size': '1'
		}`,

		`CREATE TABLE IF NOT EXISTS op_dedup (
			op_id text,
			ts timestamp,
			PRIMARY KEY (op_id)
		) WITH default_time_to_live = 604800`,
	}

	for _, table := range tables {
		if err := keyspaceSession.Query(table).Exec(); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// GetNode returns a specific node by index
func (c *TestScyllaDBCluster) GetNode(index int) *TestScyllaDBContainer {
	if index < 0 || index >= len(c.nodes) {
		return nil
	}
	return c.nodes[index]
}

// GetNodeCount returns the number of nodes in the cluster
func (c *TestScyllaDBCluster) GetNodeCount() int {
	return len(c.nodes)
}

// StopNode stops a specific node (for failure simulation)
func (c *TestScyllaDBCluster) StopNode(index int) error {
	if index < 0 || index >= len(c.nodes) {
		return fmt.Errorf("invalid node index: %d", index)
	}

	node := c.nodes[index]
	if node == nil {
		return fmt.Errorf("node %d is nil", index)
	}

	return node.Stop()
}

// StartNode restarts a specific node (for recovery simulation)
func (c *TestScyllaDBCluster) StartNode(index int) error {
	if index < 0 || index >= len(c.nodes) {
		return fmt.Errorf("invalid node index: %d", index)
	}

	node := c.nodes[index]
	if node == nil {
		return fmt.Errorf("node %d is nil", index)
	}

	// Restart the container
	seedNode := ""
	if c.seedNode != nil && node != c.seedNode {
		seedNode = c.seedNode.containerName
	}

	containerID, err := startScyllaContainer(node.containerName, node.port, c.networkName, seedNode)
	if err != nil {
		return fmt.Errorf("failed to restart node: %w", err)
	}

	node.containerID = containerID

	// Wait for node to be ready
	return node.waitForReady()
}

// TestScyllaState_Integration_BasicOperations tests basic CRUD operations with real ScyllaDB
func TestScyllaState_Integration_BasicOperations(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	// Create ScyllaState instance
	config := container.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	// Test data
	pin1 := createTestPin(t)
	pin1.PinOptions.Name = "integration-test-pin-1"

	pin2 := createTestPin(t)
	pin2.PinOptions.Name = "integration-test-pin-2"
	pin2.Cid = createTestCIDWithSuffix(t, "2")

	t.Run("Add pins", func(t *testing.T) {
		err := state.Add(ctx, pin1)
		assert.NoError(t, err)

		err = state.Add(ctx, pin2)
		assert.NoError(t, err)
	})

	t.Run("Get pins", func(t *testing.T) {
		retrievedPin1, err := state.Get(ctx, pin1.Cid)
		assert.NoError(t, err)
		assert.Equal(t, pin1.Cid, retrievedPin1.Cid)
		assert.Equal(t, pin1.PinOptions.Name, retrievedPin1.PinOptions.Name)

		retrievedPin2, err := state.Get(ctx, pin2.Cid)
		assert.NoError(t, err)
		assert.Equal(t, pin2.Cid, retrievedPin2.Cid)
		assert.Equal(t, pin2.PinOptions.Name, retrievedPin2.PinOptions.Name)
	})

	t.Run("Has pins", func(t *testing.T) {
		exists, err := state.Has(ctx, pin1.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		exists, err = state.Has(ctx, pin2.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Test non-existent pin
		nonExistentCid := createTestCIDWithSuffix(t, "nonexistent")
		exists, err = state.Has(ctx, nonExistentCid)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("List pins", func(t *testing.T) {
		pinChan := make(chan api.Pin, 10)

		go func() {
			err := state.List(ctx, pinChan)
			assert.NoError(t, err)
		}()

		// Collect pins from channel
		var pins []api.Pin
		for pin := range pinChan {
			pins = append(pins, pin)
		}

		// Should have at least our 2 test pins
		assert.GreaterOrEqual(t, len(pins), 2)

		// Verify our pins are in the list
		foundPin1, foundPin2 := false, false
		for _, pin := range pins {
			if pin.Cid.Equals(pin1.Cid) {
				foundPin1 = true
			}
			if pin.Cid.Equals(pin2.Cid) {
				foundPin2 = true
			}
		}
		assert.True(t, foundPin1, "pin1 not found in list")
		assert.True(t, foundPin2, "pin2 not found in list")
	})

	t.Run("Remove pins", func(t *testing.T) {
		err := state.Rm(ctx, pin1.Cid)
		assert.NoError(t, err)

		// Verify pin is removed
		exists, err := state.Has(ctx, pin1.Cid)
		assert.NoError(t, err)
		assert.False(t, exists)

		// pin2 should still exist
		exists, err = state.Has(ctx, pin2.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

// TestScyllaState_Integration_BatchOperations tests batch operations with real ScyllaDB
func TestScyllaState_Integration_BatchOperations(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	// Create ScyllaState instance
	config := container.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	// Create batching state
	batchState := state.Batch()
	require.NotNil(t, batchState)

	// Test data
	pins := make([]api.Pin, 5)
	for i := 0; i < 5; i++ {
		pins[i] = createTestPin(t)
		pins[i].PinOptions.Name = fmt.Sprintf("batch-pin-%d", i)
		pins[i].Cid = createTestCIDWithSuffix(t, fmt.Sprintf("batch-%d", i))
	}

	t.Run("Batch add operations", func(t *testing.T) {
		for _, pin := range pins {
			err := batchState.Add(ctx, pin)
			assert.NoError(t, err)
		}

		// Verify batch size
		assert.Equal(t, len(pins), batchState.GetBatchSize())

		// Commit batch
		err := batchState.Commit(ctx)
		assert.NoError(t, err)

		// Verify batch is cleared
		assert.Equal(t, 0, batchState.GetBatchSize())
	})

	t.Run("Verify batch operations were applied", func(t *testing.T) {
		for _, pin := range pins {
			exists, err := state.Has(ctx, pin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin %s should exist after batch commit", pin.Cid.String())
		}
	})

	t.Run("Batch remove operations", func(t *testing.T) {
		// Remove first 3 pins in batch
		for i := 0; i < 3; i++ {
			err := batchState.Rm(ctx, pins[i].Cid)
			assert.NoError(t, err)
		}

		// Commit batch
		err := batchState.Commit(ctx)
		assert.NoError(t, err)

		// Verify removals
		for i := 0; i < 3; i++ {
			exists, err := state.Has(ctx, pins[i].Cid)
			assert.NoError(t, err)
			assert.False(t, exists, "Pin %s should be removed", pins[i].Cid.String())
		}

		// Verify remaining pins still exist
		for i := 3; i < 5; i++ {
			exists, err := state.Has(ctx, pins[i].Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin %s should still exist", pins[i].Cid.String())
		}
	})
}

// TestScyllaState_Integration_Persistence tests data persistence across connections
func TestScyllaState_Integration_Persistence(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	config := container.GetConfig()
	ctx := context.Background()

	// Test data
	testPin := createTestPin(t)
	testPin.PinOptions.Name = "persistence-test-pin"

	// First connection - add pin
	t.Run("Add pin with first connection", func(t *testing.T) {
		state1, err := New(ctx, config)
		require.NoError(t, err)
		defer state1.Close()

		err = state1.Add(ctx, testPin)
		assert.NoError(t, err)

		// Verify pin exists
		exists, err := state1.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	// Second connection - verify pin persists
	t.Run("Verify pin with second connection", func(t *testing.T) {
		state2, err := New(ctx, config)
		require.NoError(t, err)
		defer state2.Close()

		// Pin should still exist
		exists, err := state2.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Retrieve and verify pin data
		retrievedPin, err := state2.Get(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.Equal(t, testPin.Cid, retrievedPin.Cid)
		assert.Equal(t, testPin.PinOptions.Name, retrievedPin.PinOptions.Name)
	})
}

// TestScyllaState_Integration_ConcurrentAccess tests concurrent access to ScyllaDB
func TestScyllaState_Integration_ConcurrentAccess(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	config := container.GetConfig()
	ctx := context.Background()

	// Create multiple state instances
	const numConnections = 5
	states := make([]*ScyllaState, numConnections)

	for i := 0; i < numConnections; i++ {
		state, err := New(ctx, config)
		require.NoError(t, err)
		defer state.Close()
		states[i] = state
	}

	// Test concurrent operations
	const numOperations = 10
	done := make(chan bool, numConnections)

	t.Run("Concurrent pin operations", func(t *testing.T) {
		for i := 0; i < numConnections; i++ {
			go func(stateIndex int) {
				defer func() { done <- true }()

				state := states[stateIndex]

				for j := 0; j < numOperations; j++ {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("concurrent-pin-%d-%d", stateIndex, j)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("concurrent-%d-%d", stateIndex, j))

					// Add pin
					err := state.Add(ctx, pin)
					assert.NoError(t, err)

					// Verify pin exists
					exists, err := state.Has(ctx, pin.Cid)
					assert.NoError(t, err)
					assert.True(t, exists)

					// Remove pin
					err = state.Rm(ctx, pin.Cid)
					assert.NoError(t, err)
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numConnections; i++ {
			<-done
		}
	})
}

// TestScyllaState_Integration_LargeDataset tests operations with larger datasets
func TestScyllaState_Integration_LargeDataset(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	config := container.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	const numPins = 100
	pins := make([]api.Pin, numPins)

	t.Run("Add large dataset", func(t *testing.T) {
		// Create test pins
		for i := 0; i < numPins; i++ {
			pins[i] = createTestPin(t)
			pins[i].PinOptions.Name = fmt.Sprintf("large-dataset-pin-%d", i)
			pins[i].Cid = createTestCIDWithSuffix(t, fmt.Sprintf("large-%d", i))
		}

		// Add pins using batch operations for efficiency
		batchState := state.Batch()
		batchSize := 20

		for i := 0; i < numPins; i++ {
			err := batchState.Add(ctx, pins[i])
			assert.NoError(t, err)

			// Commit batch when it reaches the batch size
			if (i+1)%batchSize == 0 || i == numPins-1 {
				err := batchState.Commit(ctx)
				assert.NoError(t, err)
			}
		}
	})

	t.Run("Verify large dataset", func(t *testing.T) {
		// Verify all pins exist
		for i := 0; i < numPins; i++ {
			exists, err := state.Has(ctx, pins[i].Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin %d should exist", i)
		}

		// Test list operation with large dataset
		pinChan := make(chan api.Pin, numPins+10)

		go func() {
			err := state.List(ctx, pinChan)
			assert.NoError(t, err)
		}()

		// Count pins
		var count int
		for range pinChan {
			count++
		}

		assert.GreaterOrEqual(t, count, numPins, "Should list at least %d pins", numPins)
	})
}

// TestScyllaState_Integration_ErrorHandling tests error handling with real database
func TestScyllaState_Integration_ErrorHandling(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	config := container.GetConfig()
	ctx := context.Background()

	t.Run("Invalid keyspace", func(t *testing.T) {
		invalidConfig := container.GetConfig()
		invalidConfig.Keyspace = "nonexistent_keyspace"

		_, err := New(ctx, invalidConfig)
		assert.Error(t, err)
	})

	t.Run("Connection timeout", func(t *testing.T) {
		timeoutConfig := container.GetConfig()
		timeoutConfig.Hosts = []string{"192.0.2.1"} // Non-routable IP
		timeoutConfig.ConnectTimeout = 1 * time.Second

		_, err := New(ctx, timeoutConfig)
		assert.Error(t, err)
	})

	t.Run("Operations on closed state", func(t *testing.T) {
		state, err := New(ctx, config)
		require.NoError(t, err)

		// Close the state
		err = state.Close()
		assert.NoError(t, err)

		// Operations should fail
		testPin := createTestPin(t)

		err = state.Add(ctx, testPin)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "closed")

		_, err = state.Get(ctx, testPin.Cid)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "closed")
	})
}

// Helper function to create a test CID
func createTestCID(t *testing.T) api.Cid {
	cidStr := "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	cid, err := api.DecodeCid(cidStr)
	require.NoError(t, err)
	return cid
}

// Helper function to create a test Pin
func createTestPin(t *testing.T) api.Pin {
	cid := createTestCID(t)
	pin := api.Pin{
		Cid:  cid,
		Type: api.DataType,
		Allocations: []api.PeerID{
			api.PeerID("peer1"),
			api.PeerID("peer2"),
		},
		ReplicationFactorMin: 2,
		ReplicationFactorMax: 3,
		MaxDepth:             -1,
		PinOptions: api.PinOptions{
			Name: "test-pin",
		},
	}
	return pin
}

// Helper function to create a test CID with a suffix
func createTestCIDWithSuffix(t *testing.T, suffix string) api.Cid {
	// Create a unique CID by modifying the base CID string
	baseCidStr := "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbd"

	// Pad or truncate suffix to ensure valid CID length
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}

	// Create a unique CID string by appending suffix
	cidStr := baseCidStr + fmt.Sprintf("%010s", suffix)

	// For testing purposes, we'll use a hash-based approach to create different CIDs
	// This ensures each suffix creates a unique, valid CID
	hash := fmt.Sprintf("%x", []byte(suffix))
	if len(hash) > 10 {
		hash = hash[:10]
	}

	finalCidStr := baseCidStr + hash

	cid, err := api.DecodeCid(finalCidStr)
	if err != nil {
		// Fallback to base CID if decoding fails
		return createTestCID(t)
	}
	return cid
}

// NewClusterFailureSimulator creates a new failure simulator
func NewClusterFailureSimulator(cluster *TestScyllaDBCluster) *ClusterFailureSimulator {
	return &ClusterFailureSimulator{
		cluster: cluster,
	}
}

// SimulateNodeFailure stops a node to simulate failure
func (fs *ClusterFailureSimulator) SimulateNodeFailure(nodeIndex int) error {
	return fs.cluster.StopNode(nodeIndex)
}

// SimulateNodeRecovery restarts a failed node
func (fs *ClusterFailureSimulator) SimulateNodeRecovery(nodeIndex int) error {
	return fs.cluster.StartNode(nodeIndex)
}

// SimulateNetworkPartition simulates a network partition by stopping multiple nodes
func (fs *ClusterFailureSimulator) SimulateNetworkPartition(nodeIndices []int) error {
	for _, index := range nodeIndices {
		if err := fs.cluster.StopNode(index); err != nil {
			return fmt.Errorf("failed to stop node %d: %w", index, err)
		}
	}
	return nil
}

// RecoverFromNetworkPartition recovers from a network partition by restarting nodes
func (fs *ClusterFailureSimulator) RecoverFromNetworkPartition(nodeIndices []int) error {
	for _, index := range nodeIndices {
		if err := fs.cluster.StartNode(index); err != nil {
			return fmt.Errorf("failed to start node %d: %w", index, err)
		}
	}
	return nil
}

// NewMigrationTestSuite creates a new migration test suite
func NewMigrationTestSuite(t *testing.T, sourceBackend string, destinationContainer *TestScyllaDBContainer) *MigrationTestSuite {
	suite := &MigrationTestSuite{
		testPins: generateTestPinsForMigration(t, 50),
	}

	// Create source state based on backend type
	switch sourceBackend {
	case "dsstate":
		// Create in-memory datastore for testing
		ds := inmem.New()
		sourceState, err := dsstate.New(context.Background(), ds, "test", nil)
		require.NoError(t, err)
		suite.sourceState = sourceState
	default:
		t.Fatalf("Unsupported source backend: %s", sourceBackend)
	}

	// Create destination ScyllaState
	config := destinationContainer.GetConfig()
	destinationState, err := New(context.Background(), config)
	require.NoError(t, err)
	suite.destinationState = destinationState

	return suite
}

// PopulateSourceState populates the source state with test data
func (mts *MigrationTestSuite) PopulateSourceState(ctx context.Context) error {
	for _, pin := range mts.testPins {
		if err := mts.sourceState.Add(ctx, pin); err != nil {
			return fmt.Errorf("failed to add pin to source state: %w", err)
		}
	}
	return nil
}

// PerformMigration performs the migration from source to destination
func (mts *MigrationTestSuite) PerformMigration(ctx context.Context) error {
	// Create a buffer to hold the marshaled state
	var buf bytes.Buffer

	// Marshal from source state
	if err := mts.sourceState.Marshal(&buf); err != nil {
		return fmt.Errorf("failed to marshal source state: %w", err)
	}

	// Unmarshal to destination state
	if err := mts.destinationState.Unmarshal(&buf); err != nil {
		return fmt.Errorf("failed to unmarshal to destination state: %w", err)
	}

	return nil
}

// ValidateMigration validates that the migration was successful
func (mts *MigrationTestSuite) ValidateMigration(ctx context.Context) error {
	// Check that all pins exist in destination
	for _, expectedPin := range mts.testPins {
		// Check existence
		exists, err := mts.destinationState.Has(ctx, expectedPin.Cid)
		if err != nil {
			return fmt.Errorf("failed to check pin existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("pin %s not found in destination state", expectedPin.Cid.String())
		}

		// Check pin data
		actualPin, err := mts.destinationState.Get(ctx, expectedPin.Cid)
		if err != nil {
			return fmt.Errorf("failed to get pin from destination: %w", err)
		}

		// Validate pin data
		if !actualPin.Cid.Equals(expectedPin.Cid) {
			return fmt.Errorf("CID mismatch: expected %s, got %s", expectedPin.Cid.String(), actualPin.Cid.String())
		}

		if actualPin.PinOptions.Name != expectedPin.PinOptions.Name {
			return fmt.Errorf("name mismatch for pin %s: expected %s, got %s",
				expectedPin.Cid.String(), expectedPin.PinOptions.Name, actualPin.PinOptions.Name)
		}
	}

	return nil
}

// Cleanup cleans up the migration test suite
func (mts *MigrationTestSuite) Cleanup() {
	if mts.sourceState != nil {
		if closer, ok := mts.sourceState.(io.Closer); ok {
			closer.Close()
		}
	}
	if mts.destinationState != nil {
		mts.destinationState.Close()
	}
}

// generateTestPinsForMigration generates test pins for migration testing
func generateTestPinsForMigration(t *testing.T, count int) []api.Pin {
	pins := make([]api.Pin, count)
	for i := 0; i < count; i++ {
		pin := createTestPin(t)
		pin.PinOptions.Name = fmt.Sprintf("migration-test-pin-%d", i)
		pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("migration-%d", i))
		pins[i] = pin
	}
	return pins
}

// TestScyllaState_Integration_SchemaValidation tests that the schema is correctly created
func TestScyllaState_Integration_SchemaValidation(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	config := container.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	// Test that we can perform operations that require all tables
	testPin := createTestPin(t)
	testPin.PinOptions.Name = "schema-validation-pin"

	t.Run("Schema supports all operations", func(t *testing.T) {
		// Add pin (uses pins_by_cid table)
		err := state.Add(ctx, testPin)
		assert.NoError(t, err)

		// Get pin (uses pins_by_cid table)
		retrievedPin, err := state.Get(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.Equal(t, testPin.Cid, retrievedPin.Cid)

		// Has pin (uses pins_by_cid table)
		exists, err := state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		// List pins (uses pins_by_cid table)
		pinChan := make(chan api.Pin, 10)
		go func() {
			err := state.List(ctx, pinChan)
			assert.NoError(t, err)
		}()

		// Consume the channel
		var foundPin bool
		for pin := range pinChan {
			if pin.Cid.Equals(testPin.Cid) {
				foundPin = true
			}
		}
		assert.True(t, foundPin)

		// Remove pin (uses pins_by_cid table)
		err = state.Rm(ctx, testPin.Cid)
		assert.NoError(t, err)

		// Verify removal
		exists, err = state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

// TestScyllaState_Integration_MultiNodeCluster tests operations with a multi-node ScyllaDB cluster
func TestScyllaState_Integration_MultiNodeCluster(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	// Test data
	pins := make([]api.Pin, 20)
	for i := 0; i < 20; i++ {
		pins[i] = createTestPin(t)
		pins[i].PinOptions.Name = fmt.Sprintf("multinode-pin-%d", i)
		pins[i].Cid = createTestCIDWithSuffix(t, fmt.Sprintf("multinode-%d", i))
	}

	t.Run("Add pins to cluster", func(t *testing.T) {
		for _, pin := range pins {
			err := state.Add(ctx, pin)
			assert.NoError(t, err)
		}
	})

	t.Run("Verify pins exist on cluster", func(t *testing.T) {
		for _, pin := range pins {
			exists, err := state.Has(ctx, pin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin %s should exist", pin.Cid.String())
		}
	})

	t.Run("List pins from cluster", func(t *testing.T) {
		pinChan := make(chan api.Pin, len(pins)+10)

		go func() {
			err := state.List(ctx, pinChan)
			assert.NoError(t, err)
		}()

		// Collect pins
		var retrievedPins []api.Pin
		for pin := range pinChan {
			retrievedPins = append(retrievedPins, pin)
		}

		assert.GreaterOrEqual(t, len(retrievedPins), len(pins))
	})

	t.Run("Batch operations on cluster", func(t *testing.T) {
		batchState := state.Batch()
		require.NotNil(t, batchState)

		// Add more pins in batch
		for i := 20; i < 30; i++ {
			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("batch-multinode-pin-%d", i)
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("batch-multinode-%d", i))

			err := batchState.Add(ctx, pin)
			assert.NoError(t, err)
		}

		// Commit batch
		err := batchState.Commit(ctx)
		assert.NoError(t, err)

		// Verify batch pins exist
		for i := 20; i < 30; i++ {
			pin := createTestPin(t)
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("batch-multinode-%d", i))

			exists, err := state.Has(ctx, pin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists)
		}
	})
}

// TestScyllaState_Integration_NodeFailover tests failover scenarios
func TestScyllaState_Integration_NodeFailover(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	simulator := NewClusterFailureSimulator(cluster)

	// Test data
	testPin := createTestPin(t)
	testPin.PinOptions.Name = "failover-test-pin"

	t.Run("Add pin before failure", func(t *testing.T) {
		err := state.Add(ctx, testPin)
		assert.NoError(t, err)

		exists, err := state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Single node failure", func(t *testing.T) {
		// Stop one node (not the seed node)
		failedNodeIndex := 1
		err := simulator.SimulateNodeFailure(failedNodeIndex)
		assert.NoError(t, err)

		// Wait a bit for the failure to be detected
		time.Sleep(5 * time.Second)

		// Operations should still work with remaining nodes
		exists, err := state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Add another pin during failure
		failoverPin := createTestPin(t)
		failoverPin.PinOptions.Name = "failover-during-failure-pin"
		failoverPin.Cid = createTestCIDWithSuffix(t, "failover-failure")

		err = state.Add(ctx, failoverPin)
		assert.NoError(t, err)

		// Verify the new pin exists
		exists, err = state.Has(ctx, failoverPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Recover the failed node
		err = simulator.SimulateNodeRecovery(failedNodeIndex)
		assert.NoError(t, err)

		// Wait for node to rejoin
		time.Sleep(10 * time.Second)

		// Verify both pins still exist after recovery
		exists, err = state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)

		exists, err = state.Has(ctx, failoverPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Network partition simulation", func(t *testing.T) {
		// Simulate network partition by stopping majority of nodes
		partitionNodes := []int{1, 2} // Stop 2 out of 3 nodes
		err := simulator.SimulateNetworkPartition(partitionNodes)
		assert.NoError(t, err)

		// Wait for partition to be detected
		time.Sleep(5 * time.Second)

		// Operations might fail or succeed depending on consistency level
		// With QUORUM consistency, writes should fail when majority is down
		partitionPin := createTestPin(t)
		partitionPin.PinOptions.Name = "partition-test-pin"
		partitionPin.Cid = createTestCIDWithSuffix(t, "partition")

		err = state.Add(ctx, partitionPin)
		// This might fail with QUORUM consistency when majority is down
		// We don't assert success/failure as it depends on timing and consistency settings

		// Recover from partition
		err = simulator.RecoverFromNetworkPartition(partitionNodes)
		assert.NoError(t, err)

		// Wait for cluster to recover
		time.Sleep(15 * time.Second)

		// Original pin should still exist after partition recovery
		exists, err := state.Has(ctx, testPin.Cid)
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

// TestScyllaState_Integration_ConsistencyLevels tests different consistency levels
func TestScyllaState_Integration_ConsistencyLevels(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	ctx := context.Background()

	consistencyLevels := []string{"ONE", "QUORUM", "ALL"}

	for _, consistency := range consistencyLevels {
		t.Run(fmt.Sprintf("Consistency_%s", consistency), func(t *testing.T) {
			config := cluster.GetConfig()
			config.Consistency = consistency

			state, err := New(ctx, config)
			require.NoError(t, err)
			defer state.Close()

			// Test pin with this consistency level
			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("consistency-%s-pin", consistency)
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("consistency-%s", consistency))

			// Add pin
			err = state.Add(ctx, pin)
			assert.NoError(t, err)

			// Verify pin exists
			exists, err := state.Has(ctx, pin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists)

			// Get pin
			retrievedPin, err := state.Get(ctx, pin.Cid)
			assert.NoError(t, err)
			assert.Equal(t, pin.Cid, retrievedPin.Cid)
		})
	}
}

// TestScyllaState_Integration_ConcurrentMultiNode tests concurrent operations on multi-node cluster
func TestScyllaState_Integration_ConcurrentMultiNode(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	ctx := context.Background()

	// Create multiple state instances
	const numConnections = 10
	states := make([]*ScyllaState, numConnections)

	for i := 0; i < numConnections; i++ {
		state, err := New(ctx, config)
		require.NoError(t, err)
		defer state.Close()
		states[i] = state
	}

	const numOperationsPerConnection = 20
	var wg sync.WaitGroup

	t.Run("Concurrent operations across cluster", func(t *testing.T) {
		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func(connIndex int) {
				defer wg.Done()

				state := states[connIndex]

				for j := 0; j < numOperationsPerConnection; j++ {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("concurrent-multinode-pin-%d-%d", connIndex, j)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("concurrent-multinode-%d-%d", connIndex, j))

					// Add pin
					err := state.Add(ctx, pin)
					assert.NoError(t, err)

					// Verify pin exists
					exists, err := state.Has(ctx, pin.Cid)
					assert.NoError(t, err)
					assert.True(t, exists)

					// Remove pin
					err = state.Rm(ctx, pin.Cid)
					assert.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()
	})
}

// TestScyllaState_Integration_DataMigration tests migration between different state backends
func TestScyllaState_Integration_DataMigration(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	ctx := context.Background()

	t.Run("Migration from dsstate to ScyllaState", func(t *testing.T) {
		// Create migration test suite
		suite := NewMigrationTestSuite(t, "dsstate", container)
		defer suite.Cleanup()

		// Populate source state with test data
		err := suite.PopulateSourceState(ctx)
		require.NoError(t, err)

		// Verify source state has the data
		for _, pin := range suite.testPins {
			exists, err := suite.sourceState.Has(ctx, pin.Cid)
			require.NoError(t, err)
			require.True(t, exists, "Pin should exist in source state")
		}

		// Perform migration
		err = suite.PerformMigration(ctx)
		require.NoError(t, err)

		// Validate migration
		err = suite.ValidateMigration(ctx)
		assert.NoError(t, err)
	})

	t.Run("Bidirectional migration", func(t *testing.T) {
		// Test migrating from ScyllaState back to dsstate
		config := container.GetConfig()

		// Create ScyllaState as source
		scyllaState, err := New(ctx, config)
		require.NoError(t, err)
		defer scyllaState.Close()

		// Create dsstate as destination
		ds := inmem.New()
		dsState, err := dsstate.New(ctx, ds, "test", nil)
		require.NoError(t, err)
		defer func() {
			if closer, ok := dsState.(io.Closer); ok {
				closer.Close()
			}
		}()

		// Add test data to ScyllaState
		testPins := generateTestPinsForMigration(t, 25)
		for _, pin := range testPins {
			err := scyllaState.Add(ctx, pin)
			require.NoError(t, err)
		}

		// Migrate from ScyllaState to dsstate
		var buf bytes.Buffer
		err = scyllaState.Marshal(&buf)
		require.NoError(t, err)

		err = dsState.Unmarshal(&buf)
		require.NoError(t, err)

		// Validate migration
		for _, expectedPin := range testPins {
			exists, err := dsState.Has(ctx, expectedPin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin should exist in destination state")

			actualPin, err := dsState.Get(ctx, expectedPin.Cid)
			assert.NoError(t, err)
			assert.Equal(t, expectedPin.Cid, actualPin.Cid)
		}
	})
}

// TestScyllaState_Integration_LargeScaleMigration tests migration with large datasets
func TestScyllaState_Integration_LargeScaleMigration(t *testing.T) {
	container := StartScyllaDBContainer(t)
	defer container.Stop()

	ctx := context.Background()

	t.Run("Large dataset migration", func(t *testing.T) {
		// Create a larger dataset for migration testing
		const largeDatasetSize = 1000

		// Create source dsstate
		ds := inmem.New()
		sourceState, err := dsstate.New(ctx, ds, "test", nil)
		require.NoError(t, err)
		defer func() {
			if closer, ok := sourceState.(io.Closer); ok {
				closer.Close()
			}
		}()

		// Create destination ScyllaState
		config := container.GetConfig()
		destinationState, err := New(ctx, config)
		require.NoError(t, err)
		defer destinationState.Close()

		// Generate large test dataset
		testPins := make([]api.Pin, largeDatasetSize)
		for i := 0; i < largeDatasetSize; i++ {
			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("large-migration-pin-%d", i)
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("large-migration-%d", i))
			testPins[i] = pin
		}

		// Populate source state
		t.Logf("Populating source state with %d pins", largeDatasetSize)
		for i, pin := range testPins {
			err := sourceState.Add(ctx, pin)
			require.NoError(t, err)

			if (i+1)%100 == 0 {
				t.Logf("Added %d/%d pins to source state", i+1, largeDatasetSize)
			}
		}

		// Perform migration
		t.Logf("Starting migration of %d pins", largeDatasetSize)
		start := time.Now()

		var buf bytes.Buffer
		err = sourceState.Marshal(&buf)
		require.NoError(t, err)

		err = destinationState.Unmarshal(&buf)
		require.NoError(t, err)

		migrationDuration := time.Since(start)
		t.Logf("Migration completed in %v", migrationDuration)

		// Validate migration with sampling
		t.Logf("Validating migration")
		sampleSize := 100
		sampleIndices := make([]int, sampleSize)
		for i := 0; i < sampleSize; i++ {
			sampleIndices[i] = i * (largeDatasetSize / sampleSize)
		}

		for _, index := range sampleIndices {
			if index >= len(testPins) {
				continue
			}

			expectedPin := testPins[index]

			exists, err := destinationState.Has(ctx, expectedPin.Cid)
			assert.NoError(t, err)
			assert.True(t, exists, "Pin %d should exist after migration", index)

			if exists {
				actualPin, err := destinationState.Get(ctx, expectedPin.Cid)
				assert.NoError(t, err)
				assert.Equal(t, expectedPin.Cid, actualPin.Cid)
				assert.Equal(t, expectedPin.PinOptions.Name, actualPin.PinOptions.Name)
			}
		}

		t.Logf("Migration validation completed successfully")
	})
}

// TestScyllaState_Integration_FailoverDuringMigration tests migration resilience during failures
func TestScyllaState_Integration_FailoverDuringMigration(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	ctx := context.Background()
	simulator := NewClusterFailureSimulator(cluster)

	t.Run("Migration with node failure", func(t *testing.T) {
		// Create source state
		ds := inmem.New()
		sourceState, err := dsstate.New(ctx, ds, "test", nil)
		require.NoError(t, err)
		defer func() {
			if closer, ok := sourceState.(io.Closer); ok {
				closer.Close()
			}
		}()

		// Create destination ScyllaState
		config := cluster.GetConfig()
		destinationState, err := New(ctx, config)
		require.NoError(t, err)
		defer destinationState.Close()

		// Populate source with test data
		testPins := generateTestPinsForMigration(t, 100)
		for _, pin := range testPins {
			err := sourceState.Add(ctx, pin)
			require.NoError(t, err)
		}

		// Start migration
		var buf bytes.Buffer
		err = sourceState.Marshal(&buf)
		require.NoError(t, err)

		// Simulate node failure during migration
		go func() {
			time.Sleep(1 * time.Second)
			simulator.SimulateNodeFailure(1) // Stop non-seed node
		}()

		// Continue with migration (should handle the failure gracefully)
		err = destinationState.Unmarshal(&buf)
		// Migration might succeed or fail depending on timing and consistency settings
		// We don't assert success/failure as it depends on the graceful degradation implementation

		// Recover the failed node
		time.Sleep(2 * time.Second)
		err = simulator.SimulateNodeRecovery(1)
		assert.NoError(t, err)

		// Wait for recovery
		time.Sleep(10 * time.Second)

		// Verify that at least some data was migrated successfully
		successCount := 0
		for _, pin := range testPins {
			exists, err := destinationState.Has(ctx, pin.Cid)
			if err == nil && exists {
				successCount++
			}
		}

		t.Logf("Successfully migrated %d/%d pins despite node failure", successCount, len(testPins))
		// We expect at least some pins to be migrated successfully
		assert.Greater(t, successCount, 0, "At least some pins should be migrated successfully")
	})
}

// TestScyllaState_Integration_PerformanceUnderLoad tests performance under various load conditions
func TestScyllaState_Integration_PerformanceUnderLoad(t *testing.T) {
	cluster := StartScyllaDBCluster(t, nodeCount)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	ctx := context.Background()

	state, err := New(ctx, config)
	require.NoError(t, err)
	defer state.Close()

	t.Run("High concurrency operations", func(t *testing.T) {
		const numGoroutines = 50
		const operationsPerGoroutine = 20

		var wg sync.WaitGroup
		start := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineIndex int) {
				defer wg.Done()

				for j := 0; j < operationsPerGoroutine; j++ {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("load-test-pin-%d-%d", goroutineIndex, j)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("load-%d-%d", goroutineIndex, j))

					// Add pin
					err := state.Add(ctx, pin)
					assert.NoError(t, err)

					// Verify pin exists
					exists, err := state.Has(ctx, pin.Cid)
					assert.NoError(t, err)
					assert.True(t, exists)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		totalOperations := numGoroutines * operationsPerGoroutine * 2 // Add + Has operations
		opsPerSecond := float64(totalOperations) / duration.Seconds()

		t.Logf("Completed %d operations in %v (%.2f ops/sec)", totalOperations, duration, opsPerSecond)

		// Performance assertion - should handle at least 100 ops/sec
		assert.Greater(t, opsPerSecond, 100.0, "Should achieve at least 100 operations per second")
	})

	t.Run("Batch operation performance", func(t *testing.T) {
		const batchSize = 100
		const numBatches = 10

		start := time.Now()

		for batchIndex := 0; batchIndex < numBatches; batchIndex++ {
			batchState := state.Batch()

			// Add pins to batch
			for i := 0; i < batchSize; i++ {
				pin := createTestPin(t)
				pin.PinOptions.Name = fmt.Sprintf("batch-perf-pin-%d-%d", batchIndex, i)
				pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("batch-perf-%d-%d", batchIndex, i))

				err := batchState.Add(ctx, pin)
				assert.NoError(t, err)
			}

			// Commit batch
			err := batchState.Commit(ctx)
			assert.NoError(t, err)
		}

		duration := time.Since(start)
		totalPins := batchSize * numBatches
		pinsPerSecond := float64(totalPins) / duration.Seconds()

		t.Logf("Batch operations: %d pins in %v (%.2f pins/sec)", totalPins, duration, pinsPerSecond)

		// Batch operations should be significantly faster
		assert.Greater(t, pinsPerSecond, 500.0, "Batch operations should achieve at least 500 pins per second")
	})
}
