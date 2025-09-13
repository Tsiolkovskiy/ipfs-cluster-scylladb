package scyllastate

import (
	"time"
)

// mhPrefix extracts the first 2 bytes of a multihash for partitioning
// This provides even distribution across ScyllaDB partitions
func mhPrefix(cidBin []byte) int16 {
	if len(cidBin) < 2 {
		return 0
	}
	return int16(cidBin[0])<<8 | int16(cidBin[1])
}

// cidToPartitionedKey converts CID to partitioned key components
func cidToPartitionedKey(cidBin []byte) (int16, []byte) {
	prefix := mhPrefix(cidBin)
	return prefix, cidBin
}

// toSet converts a string slice to a set for ScyllaDB
func toSet(items []string) []string {
	if items == nil {
		return []string{}
	}
	// Remove duplicates
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// toTS converts unix milliseconds to timestamp, handling nil
func toTS(unixMs *int64) *time.Time {
	if unixMs == nil {
		return nil
	}
	ts := time.Unix(0, *unixMs*int64(time.Millisecond)).UTC()
	return &ts
}

// fromTS converts timestamp to unix milliseconds, handling nil
func fromTS(ts *time.Time) *int64 {
	if ts == nil {
		return nil
	}
	ms := ts.UnixNano() / int64(time.Millisecond)
	return &ms
}

// truncateToHour truncates a unix timestamp to the hour boundary
// Used for TTL queue bucketing
func truncateToHour(unixMs int64) time.Time {
	ts := time.Unix(0, unixMs*int64(time.Millisecond)).UTC()
	return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), 0, 0, 0, time.UTC)
}

// validateCID performs basic validation on binary CID
func validateCID(cidBin []byte) error {
	if len(cidBin) == 0 {
		return ErrInvalidCID
	}
	// Add more validation as needed
	return nil
}

// validatePeerID performs basic validation on peer ID
func validatePeerID(peerID string) error {
	if peerID == "" {
		return ErrInvalidPeerID
	}
	// Add more validation as needed (e.g., multihash format)
	return nil
}

// validateOpID performs basic validation on operation ID
func validateOpID(opID string) error {
	if opID == "" {
		return ErrInvalidOpID
	}
	// Should be ULID or UUID format
	if len(opID) < 16 {
		return ErrInvalidOpID
	}
	return nil
}
