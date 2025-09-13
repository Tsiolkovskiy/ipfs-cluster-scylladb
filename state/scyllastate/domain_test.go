package scyllastate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPinState_String(t *testing.T) {
	tests := []struct {
		state    PinState
		expected string
	}{
		{StateQueued, "queued"},
		{StatePinning, "pinning"},
		{StatePinned, "pinned"},
		{StateFailed, "failed"},
		{StateUnpinned, "unpinned"},
		{PinState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestPinSpec_Validation(t *testing.T) {
	now := time.Now().UnixMilli()

	validSpec := PinSpec{
		CIDBin:   []byte{0x12, 0x20, 0x01, 0x02, 0x03}, // Valid multihash
		Owner:    "test-owner",
		RF:       3,
		PinType:  1,
		Tags:     []string{"test", "video"},
		TTL:      &now,
		Metadata: map[string]string{"filename": "test.mp4"},
	}

	assert.NotEmpty(t, validSpec.CIDBin)
	assert.NotEmpty(t, validSpec.Owner)
	assert.Greater(t, validSpec.RF, uint8(0))
	assert.NotNil(t, validSpec.TTL)
}

func TestAssignment_Basic(t *testing.T) {
	assignment := Assignment{
		CIDBin:  []byte{0x12, 0x20, 0x01, 0x02, 0x03},
		Desired: []string{"peer1", "peer2", "peer3"},
	}

	assert.NotEmpty(t, assignment.CIDBin)
	assert.Len(t, assignment.Desired, 3)
	assert.Contains(t, assignment.Desired, "peer1")
}

func TestPinPlacement_Basic(t *testing.T) {
	now := time.Now()
	placement := PinPlacement{
		CIDBin:    []byte{0x12, 0x20, 0x01, 0x02, 0x03},
		Desired:   []string{"peer1", "peer2", "peer3"},
		Actual:    []string{"peer1", "peer2"},
		UpdatedAt: now,
	}

	assert.NotEmpty(t, placement.CIDBin)
	assert.Len(t, placement.Desired, 3)
	assert.Len(t, placement.Actual, 2)
	assert.Equal(t, now, placement.UpdatedAt)
}

func TestPeerPin_Basic(t *testing.T) {
	now := time.Now()
	peerPin := PeerPin{
		PeerID:   "peer1",
		CIDBin:   []byte{0x12, 0x20, 0x01, 0x02, 0x03},
		State:    StatePinned,
		LastSeen: now,
	}

	assert.Equal(t, "peer1", peerPin.PeerID)
	assert.NotEmpty(t, peerPin.CIDBin)
	assert.Equal(t, StatePinned, peerPin.State)
	assert.Equal(t, now, peerPin.LastSeen)
}

func TestTTLEntry_Basic(t *testing.T) {
	now := time.Now()
	bucket := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)

	entry := TTLEntry{
		TTLBucket: bucket,
		CIDBin:    []byte{0x12, 0x20, 0x01, 0x02, 0x03},
		Owner:     "test-owner",
		TTL:       now,
	}

	assert.Equal(t, bucket, entry.TTLBucket)
	assert.NotEmpty(t, entry.CIDBin)
	assert.Equal(t, "test-owner", entry.Owner)
	assert.Equal(t, now, entry.TTL)
}
