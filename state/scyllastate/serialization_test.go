package scyllastate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

// TestSerializePin tests the pin serialization functionality
func TestSerializePin(t *testing.T) {
	s := &ScyllaState{}

	// Create a test pin with various fields populated
	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	testPeer, err := peer.Decode("12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf")
	require.NoError(t, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: 2,
			ReplicationFactorMax: 3,
			Name:                 "test-pin",
			Metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			ExpireAt: time.Now().Add(24 * time.Hour),
		},
		Allocations: []peer.ID{testPeer},
		MaxDepth:    -1,
		Timestamp:   time.Now(),
	}

	// Test serialization
	data, err := s.serializePin(pin)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test deserialization
	deserializedPin, err := s.deserializePin(pin.Cid, data)
	require.NoError(t, err)

	// Verify the deserialized pin matches the original
	assert.True(t, pin.Cid.Equals(deserializedPin.Cid))
	assert.Equal(t, pin.Type, deserializedPin.Type)
	assert.Equal(t, pin.ReplicationFactorMin, deserializedPin.ReplicationFactorMin)
	assert.Equal(t, pin.ReplicationFactorMax, deserializedPin.ReplicationFactorMax)
	assert.Equal(t, pin.Name, deserializedPin.Name)
	assert.Equal(t, pin.Metadata, deserializedPin.Metadata)
	assert.Equal(t, len(pin.Allocations), len(deserializedPin.Allocations))
	if len(pin.Allocations) > 0 {
		assert.Equal(t, pin.Allocations[0], deserializedPin.Allocations[0])
	}
	assert.Equal(t, pin.MaxDepth, deserializedPin.MaxDepth)

	// Timestamps might have slight differences due to precision, so check within a second
	assert.WithinDuration(t, pin.Timestamp, deserializedPin.Timestamp, time.Second)
	assert.WithinDuration(t, pin.ExpireAt, deserializedPin.ExpireAt, time.Second)
}

// TestSerializePinEmpty tests serialization of an empty pin
func TestSerializePinEmpty(t *testing.T) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
	}

	// Test serialization
	data, err := s.serializePin(pin)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test deserialization
	deserializedPin, err := s.deserializePin(pin.Cid, data)
	require.NoError(t, err)

	// Verify basic fields
	assert.True(t, pin.Cid.Equals(deserializedPin.Cid))
	assert.Equal(t, pin.Type, deserializedPin.Type)
}

// TestDeserializePinVersionCompatibility tests backward compatibility with different versions
func TestDeserializePinVersionCompatibility(t *testing.T) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
		PinOptions: api.PinOptions{
			Name: "test-pin",
		},
	}

	// Test with current version
	data, err := s.serializePin(pin)
	require.NoError(t, err)

	deserializedPin, err := s.deserializePin(pin.Cid, data)
	require.NoError(t, err)
	assert.Equal(t, pin.Name, deserializedPin.Name)

	// Test with raw protobuf data (simulating old format)
	rawProtobufData, err := pin.ProtoMarshal()
	require.NoError(t, err)

	deserializedPin2, err := s.deserializePin(pin.Cid, rawProtobufData)
	require.NoError(t, err)
	assert.Equal(t, pin.Name, deserializedPin2.Name)
}

// TestValidateSerializedPin tests the validation functionality
func TestValidateSerializedPin(t *testing.T) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
	}

	// Test with valid data
	data, err := s.serializePin(pin)
	require.NoError(t, err)

	err = s.validateSerializedPin(pin.Cid, data)
	assert.NoError(t, err)

	// Test with invalid data
	err = s.validateSerializedPin(pin.Cid, []byte("invalid data"))
	assert.Error(t, err)

	// Test with empty data
	err = s.validateSerializedPin(pin.Cid, []byte{})
	assert.Error(t, err)
}

// TestMigrateSerializedPin tests the migration functionality
func TestMigrateSerializedPin(t *testing.T) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
		PinOptions: api.PinOptions{
			Name: "test-pin",
		},
	}

	// Create old format data (raw protobuf)
	oldData, err := pin.ProtoMarshal()
	require.NoError(t, err)

	// Migrate to new format
	newData, err := s.migrateSerializedPin(pin.Cid, oldData)
	require.NoError(t, err)
	assert.NotEqual(t, oldData, newData) // Should be different formats

	// Verify the migrated data can be deserialized correctly
	deserializedPin, err := s.deserializePin(pin.Cid, newData)
	require.NoError(t, err)
	assert.Equal(t, pin.Name, deserializedPin.Name)
	assert.True(t, pin.Cid.Equals(deserializedPin.Cid))
}

// TestSerializationVersionHandling tests handling of different serialization versions
func TestSerializationVersionHandling(t *testing.T) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	// Test with unsupported version
	unsupportedVersionData := SerializedPin{
		Version: 99, // Unsupported version
		Data:    []byte("some data"),
	}

	unsupportedData, err := json.Marshal(unsupportedVersionData)
	require.NoError(t, err)

	_, err = s.deserializePin(api.NewCid(testCid), unsupportedData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pin serialization version")
}

// BenchmarkSerializePin benchmarks the pin serialization performance
func BenchmarkSerializePin(b *testing.B) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(b, err)

	testPeer, err := peer.Decode("12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf")
	require.NoError(b, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: 2,
			ReplicationFactorMax: 3,
			Name:                 "benchmark-pin",
			Metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		Allocations: []peer.ID{testPeer},
		MaxDepth:    -1,
		Timestamp:   time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.serializePin(pin)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDeserializePin benchmarks the pin deserialization performance
func BenchmarkDeserializePin(b *testing.B) {
	s := &ScyllaState{}

	testCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(b, err)

	testPeer, err := peer.Decode("12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf")
	require.NoError(b, err)

	pin := api.Pin{
		Cid:  api.NewCid(testCid),
		Type: api.DataType,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: 2,
			ReplicationFactorMax: 3,
			Name:                 "benchmark-pin",
			Metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		Allocations: []peer.ID{testPeer},
		MaxDepth:    -1,
		Timestamp:   time.Now(),
	}

	data, err := s.serializePin(pin)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.deserializePin(pin.Cid, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
