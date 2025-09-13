// Package scyllastate implements IPFS-Cluster state interface using ScyllaDB
package scyllastate

import (
	"time"
)

// PinState represents the state of a pin on a peer
type PinState uint8

const (
	StateQueued PinState = iota
	StatePinning
	StatePinned
	StateFailed
	StateUnpinned
)

// String returns string representation of PinState
func (s PinState) String() string {
	switch s {
	case StateQueued:
		return "queued"
	case StatePinning:
		return "pinning"
	case StatePinned:
		return "pinned"
	case StateFailed:
		return "failed"
	case StateUnpinned:
		return "unpinned"
	default:
		return "unknown"
	}
}

// PinSpec represents a pin specification for storage
type PinSpec struct {
	CIDBin   []byte            // Binary multihash (without base58/base32 prefixes)
	Owner    string            // Tenant/owner identifier
	RF       uint8             // Required replication factor
	PinType  uint8             // 0=direct, 1=recursive
	Tags     []string          // Arbitrary tags
	TTL      *int64            // Planned auto-removal (unix ms, nil = permanent)
	Metadata map[string]string // Additional metadata
}

// Assignment represents desired placement for a CID
type Assignment struct {
	CIDBin  []byte   // Binary CID
	Desired []string // List of peer IDs where pin should be placed
}

// PinPlacement represents current placement state
type PinPlacement struct {
	CIDBin    []byte    // Binary CID
	Desired   []string  // Desired peer IDs
	Actual    []string  // Actual peer IDs where pin is confirmed
	UpdatedAt time.Time // Last update timestamp
}

// PeerPin represents a pin on a specific peer
type PeerPin struct {
	PeerID   string    // Peer identifier
	CIDBin   []byte    // Binary CID
	State    PinState  // Current state
	LastSeen time.Time // Last status update
}

// TTLEntry represents an entry in TTL queue
type TTLEntry struct {
	TTLBucket time.Time // Hourly bucket for efficient scanning
	CIDBin    []byte    // Binary CID
	Owner     string    // Owner identifier
	TTL       time.Time // Exact TTL timestamp
}
