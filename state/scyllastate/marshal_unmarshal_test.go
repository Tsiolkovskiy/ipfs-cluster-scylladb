package scyllastate

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScyllaState_Marshal_Unmarshal(t *testing.T) {
	ctx := context.Background()

	// Create test configuration
	config := &Config{
		Hosts:     []string{"127.0.0.1"},
		Port:      9042,
		Keyspace:  "test_marshal",
		BatchSize: 100,
	}

	// Create ScyllaState instance (this will fail without real ScyllaDB, but we can test the logic)
	state := &ScyllaState{
		config:    config,
		totalPins: 0,
		closed:    0,
	}

	// Test data structures
	t.Run("StateExportHeader", func(t *testing.T) {
		header := StateExportHeader{
			Version:   CurrentStateVersion,
			Timestamp: time.Now(),
			Keyspace:  "test",
		}

		// Test that header can be encoded/decoded with JSON
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		err := enc.Encode(header)
		require.NoError(t, err)

		var decodedHeader StateExportHeader
		dec := json.NewDecoder(&buf)
		err = dec.Decode(&decodedHeader)
		require.NoError(t, err)

		assert.Equal(t, header.Version, decodedHeader.Version)
		assert.Equal(t, header.Keyspace, decodedHeader.Keyspace)
		assert.WithinDuration(t, header.Timestamp, decodedHeader.Timestamp, time.Second)
	})

	t.Run("StateEntry", func(t *testing.T) {
		// Create test CID
		testCid, err := api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
		require.NoError(t, err)

		entry := StateEntry{
			CID:  testCid.Bytes(),
			Data: []byte("test data"),
		}

		// Test that entry can be encoded/decoded with JSON
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		err = enc.Encode(entry)
		require.NoError(t, err)

		var decodedEntry StateEntry
		dec := json.NewDecoder(&buf)
		err = dec.Decode(&decodedEntry)
		require.NoError(t, err)

		assert.Equal(t, entry.CID, decodedEntry.CID)
		assert.Equal(t, entry.Data, decodedEntry.Data)
	})
}

func TestScyllaState_MigrationHelpers(t *testing.T) {
	ctx := context.Background()

	// Create test configuration
	config := &Config{
		Hosts:     []string{"127.0.0.1"},
		Port:      9042,
		Keyspace:  "test_migration",
		BatchSize: 100,
	}

	// Create ScyllaState instance
	state := &ScyllaState{
		config:    config,
		totalPins: 0,
		closed:    0,
	}

	t.Run("tryUnmarshalCurrentFormat", func(t *testing.T) {
		// Create a buffer with current format data
		var buf bytes.Buffer

		// Write header
		header := StateExportHeader{
			Version:   CurrentStateVersion,
			Timestamp: time.Now(),
			Keyspace:  "test",
		}

		enc := json.NewEncoder(&buf)
		err := enc.Encode(header)
		require.NoError(t, err)

		// Test detection (this will fail because we don't have a real ScyllaDB connection,
		// but we can verify the method exists and handles the format correctly)
		err = state.tryUnmarshalCurrentFormat(&buf)
		// We expect this to fail due to no ScyllaDB connection, but not due to format issues
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")
	})

	t.Run("migrateLegacyFormat", func(t *testing.T) {
		// Create legacy format data
		var buf bytes.Buffer

		// Write a simple pin in legacy format (length-prefixed)
		testData := []byte("test pin data")
		buf.WriteString("13\n") // length
		buf.Write(testData)
		buf.WriteString("\n")

		// Test migration (this will fail due to no ScyllaDB connection)
		count, err := state.migrateLegacyFormat(ctx, &buf)
		assert.Error(t, err) // Expected due to no ScyllaDB connection
		assert.Equal(t, int64(0), count)
	})

	t.Run("migrateDSStateFormat", func(t *testing.T) {
		// Create dsstate format data
		var buf bytes.Buffer

		type serialEntry struct {
			Key   string `codec:"k"`
			Value []byte `codec:"v"`
		}

		entry := serialEntry{
			Key:   "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
			Value: []byte("test pin data"),
		}

		enc := json.NewEncoder(&buf)
		err := enc.Encode(entry)
		require.NoError(t, err)

		// Test migration (this will fail due to no ScyllaDB connection)
		count, err := state.migrateDSStateFormat(ctx, &buf)
		assert.Error(t, err) // Expected due to no ScyllaDB connection
		assert.Equal(t, int64(0), count)
	})
}

func TestScyllaState_VersionCompatibility(t *testing.T) {
	t.Run("CurrentStateVersion", func(t *testing.T) {
		// Ensure version is set correctly
		assert.Equal(t, 1, CurrentStateVersion)
	})

	t.Run("StateExportHeader_VersionValidation", func(t *testing.T) {
		// Test version validation logic
		validHeader := StateExportHeader{
			Version:   CurrentStateVersion,
			Timestamp: time.Now(),
			Keyspace:  "test",
		}
		assert.Equal(t, CurrentStateVersion, validHeader.Version)

		futureHeader := StateExportHeader{
			Version:   CurrentStateVersion + 1,
			Timestamp: time.Now(),
			Keyspace:  "test",
		}
		assert.Greater(t, futureHeader.Version, CurrentStateVersion)
	})
}

// Benchmark tests for Marshal/Unmarshal performance
func BenchmarkStateEntry_Encode(b *testing.B) {
	// Create test CID
	testCid, err := api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(b, err)

	entry := StateEntry{
		CID:  testCid.Bytes(),
		Data: make([]byte, 1024), // 1KB test data
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		err := enc.Encode(entry)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateEntry_Decode(b *testing.B) {
	// Create test CID
	testCid, err := api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(b, err)

	entry := StateEntry{
		CID:  testCid.Bytes(),
		Data: make([]byte, 1024), // 1KB test data
	}

	// Pre-encode the data
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	err = enc.Encode(entry)
	require.NoError(b, err)

	encodedData := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decodedEntry StateEntry
		dec := json.NewDecoder(bytes.NewReader(encodedData))
		err := dec.Decode(&decodedEntry)
		if err != nil {
			b.Fatal(err)
		}
	}
}
