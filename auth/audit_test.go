package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileAuditLogger(t *testing.T) {
	// Create temporary log file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")
	
	logger, err := NewFileAuditLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()
	
	// Test logging access event
	accessEvent := &AccessEvent{
		Timestamp: time.Now(),
		Principal: &Principal{
			ID:       "test-user",
			Type:     "user",
			TenantID: "default",
			Roles:    []string{"user"},
			Method:   AuthMethodJWT,
		},
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
		Result:    AccessGranted,
		RequestID: "req-123",
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
		Context:   map[string]string{"cid": "QmTest"},
	}
	
	err = logger.LogAccess(context.Background(), accessEvent)
	require.NoError(t, err)
	
	// Test logging auth event
	authEvent := &AuthEvent{
		Timestamp: time.Now(),
		Method:    AuthMethodJWT,
		Result:    AuthSuccess,
		Principal: accessEvent.Principal,
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	err = logger.LogAuth(context.Background(), authEvent)
	require.NoError(t, err)
	
	// Close logger to flush data
	err = logger.Close()
	require.NoError(t, err)
	
	// Verify log file contents
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	
	// Verify log file has content
	
	// Should have at least some content
	assert.Greater(t, len(data), 0)
}

func TestFileAuditLogger_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "subdir", "audit.log")
	
	logger, err := NewFileAuditLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()
	
	// Verify directory was created
	assert.DirExists(t, filepath.Dir(logPath))
}

func TestMemoryAuditLogger(t *testing.T) {
	logger := NewMemoryAuditLogger()
	
	// Test logging access event
	accessEvent := &AccessEvent{
		Timestamp: time.Now(),
		Principal: &Principal{
			ID:       "test-user",
			Type:     "user",
			TenantID: "default",
			Roles:    []string{"user"},
			Method:   AuthMethodJWT,
		},
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
		Result:    AccessGranted,
		RequestID: "req-123",
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
		Context:   map[string]string{"cid": "QmTest"},
	}
	
	err := logger.LogAccess(context.Background(), accessEvent)
	require.NoError(t, err)
	
	// Test logging auth event
	authEvent := &AuthEvent{
		Timestamp: time.Now(),
		Method:    AuthMethodJWT,
		Result:    AuthSuccess,
		Principal: accessEvent.Principal,
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	err = logger.LogAuth(context.Background(), authEvent)
	require.NoError(t, err)
	
	// Verify events were logged
	accessEvents := logger.GetAccessEvents()
	assert.Len(t, accessEvents, 1)
	assert.Equal(t, accessEvent.Principal.ID, accessEvents[0].Principal.ID)
	assert.Equal(t, accessEvent.Resource, accessEvents[0].Resource)
	assert.Equal(t, accessEvent.Action, accessEvents[0].Action)
	assert.Equal(t, accessEvent.Result, accessEvents[0].Result)
	
	authEvents := logger.GetAuthEvents()
	assert.Len(t, authEvents, 1)
	assert.Equal(t, authEvent.Method, authEvents[0].Method)
	assert.Equal(t, authEvent.Result, authEvents[0].Result)
	assert.Equal(t, authEvent.Principal.ID, authEvents[0].Principal.ID)
	
	// Test clear
	logger.Clear()
	assert.Len(t, logger.GetAccessEvents(), 0)
	assert.Len(t, logger.GetAuthEvents(), 0)
}

func TestMemoryAuditLogger_Concurrent(t *testing.T) {
	logger := NewMemoryAuditLogger()
	
	// Test concurrent access
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			accessEvent := &AccessEvent{
				Timestamp: time.Now(),
				Principal: &Principal{
					ID:       "test-user",
					Type:     "user",
					TenantID: "default",
					Roles:    []string{"user"},
					Method:   AuthMethodJWT,
				},
				Resource: ResourcePin,
				Action:   ActionPinAdd,
				Result:   AccessGranted,
			}
			
			err := logger.LogAccess(context.Background(), accessEvent)
			assert.NoError(t, err)
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify all events were logged
	events := logger.GetAccessEvents()
	assert.Len(t, events, 10)
}

func TestAuditEventSerialization(t *testing.T) {
	// Test AccessEvent JSON serialization
	accessEvent := &AccessEvent{
		Timestamp: time.Now(),
		Principal: &Principal{
			ID:       "test-user",
			Type:     "user",
			TenantID: "default",
			Roles:    []string{"user", "admin"},
			Method:   AuthMethodJWT,
		},
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
		Result:    AccessGranted,
		RequestID: "req-123",
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
		Context:   map[string]string{"cid": "QmTest", "size": "1024"},
	}
	
	data, err := json.Marshal(accessEvent)
	require.NoError(t, err)
	
	var decoded AccessEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, accessEvent.Principal.ID, decoded.Principal.ID)
	assert.Equal(t, accessEvent.Resource, decoded.Resource)
	assert.Equal(t, accessEvent.Action, decoded.Action)
	assert.Equal(t, accessEvent.Result, decoded.Result)
	assert.Equal(t, accessEvent.Context, decoded.Context)
	
	// Test AuthEvent JSON serialization
	authEvent := &AuthEvent{
		Timestamp: time.Now(),
		Method:    AuthMethodJWT,
		Result:    AuthSuccess,
		Principal: accessEvent.Principal,
		ClientIP:  "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	data, err = json.Marshal(authEvent)
	require.NoError(t, err)
	
	var decodedAuth AuthEvent
	err = json.Unmarshal(data, &decodedAuth)
	require.NoError(t, err)
	
	assert.Equal(t, authEvent.Method, decodedAuth.Method)
	assert.Equal(t, authEvent.Result, decodedAuth.Result)
	assert.Equal(t, authEvent.Principal.ID, decodedAuth.Principal.ID)
}