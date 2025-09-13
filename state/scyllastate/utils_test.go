package scyllastate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMhPrefix(t *testing.T) {
	tests := []struct {
		name     string
		cidBin   []byte
		expected int16
	}{
		{
			name:     "normal multihash",
			cidBin:   []byte{0x12, 0x34, 0x56, 0x78},
			expected: 0x1234,
		},
		{
			name:     "single byte",
			cidBin:   []byte{0xFF},
			expected: 0,
		},
		{
			name:     "empty",
			cidBin:   []byte{},
			expected: 0,
		},
		{
			name:     "zero bytes",
			cidBin:   []byte{0x00, 0x00},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mhPrefix(tt.cidBin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToSet(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toSet(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestToTS(t *testing.T) {
	tests := []struct {
		name     string
		unixMs   *int64
		expected *time.Time
	}{
		{
			name:     "nil input",
			unixMs:   nil,
			expected: nil,
		},
		{
			name:     "zero timestamp",
			unixMs:   func() *int64 { v := int64(0); return &v }(),
			expected: func() *time.Time { t := time.Unix(0, 0).UTC(); return &t }(),
		},
		{
			name:     "normal timestamp",
			unixMs:   func() *int64 { v := int64(1640995200000); return &v }(), // 2022-01-01 00:00:00 UTC
			expected: func() *time.Time { t := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTS(tt.unixMs)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestFromTS(t *testing.T) {
	tests := []struct {
		name     string
		ts       *time.Time
		expected *int64
	}{
		{
			name:     "nil input",
			ts:       nil,
			expected: nil,
		},
		{
			name:     "zero timestamp",
			ts:       func() *time.Time { t := time.Unix(0, 0).UTC(); return &t }(),
			expected: func() *int64 { v := int64(0); return &v }(),
		},
		{
			name:     "normal timestamp",
			ts:       func() *time.Time { t := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
			expected: func() *int64 { v := int64(1640995200000); return &v }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromTS(tt.ts)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestTruncateToHour(t *testing.T) {
	tests := []struct {
		name     string
		unixMs   int64
		expected time.Time
	}{
		{
			name:     "beginning of hour",
			unixMs:   1640995200000, // 2022-01-01 00:00:00 UTC
			expected: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "middle of hour",
			unixMs:   1640997030000, // 2022-01-01 00:30:30 UTC
			expected: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "end of hour",
			unixMs:   1640998799999, // 2022-01-01 00:59:59.999 UTC
			expected: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateToHour(tt.unixMs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateCID(t *testing.T) {
	tests := []struct {
		name    string
		cidBin  []byte
		wantErr bool
	}{
		{
			name:    "valid CID",
			cidBin:  []byte{0x12, 0x20, 0x01, 0x02, 0x03},
			wantErr: false,
		},
		{
			name:    "empty CID",
			cidBin:  []byte{},
			wantErr: true,
		},
		{
			name:    "nil CID",
			cidBin:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCID(tt.cidBin)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePeerID(t *testing.T) {
	tests := []struct {
		name    string
		peerID  string
		wantErr bool
	}{
		{
			name:    "valid peer ID",
			peerID:  "QmPeer123",
			wantErr: false,
		},
		{
			name:    "empty peer ID",
			peerID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePeerID(tt.peerID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateOpID(t *testing.T) {
	tests := []struct {
		name    string
		opID    string
		wantErr bool
	}{
		{
			name:    "valid ULID",
			opID:    "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			wantErr: false,
		},
		{
			name:    "valid UUID",
			opID:    "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "empty op ID",
			opID:    "",
			wantErr: true,
		},
		{
			name:    "too short",
			opID:    "short",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOpID(tt.opID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
