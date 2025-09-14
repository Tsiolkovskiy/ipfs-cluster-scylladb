package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileAuditLogger implements AuditLogger using file storage
type FileAuditLogger struct {
	logPath string
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewFileAuditLogger creates a new file-based audit logger
func NewFileAuditLogger(logPath string) (*FileAuditLogger, error) {
	if logPath == "" {
		logPath = "/var/log/ipfs-cluster/audit.log"
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	
	// Open log file in append mode
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}
	
	return &FileAuditLogger{
		logPath: logPath,
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

// LogAccess logs an access event to file
func (f *FileAuditLogger) LogAccess(ctx context.Context, event *AccessEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	logEntry := map[string]interface{}{
		"type":      "access",
		"timestamp": event.Timestamp.Format(time.RFC3339),
		"resource":  event.Resource,
		"action":    event.Action,
		"result":    string(event.Result),
	}
	
	if event.Principal != nil {
		logEntry["principal"] = map[string]interface{}{
			"id":        event.Principal.ID,
			"type":      event.Principal.Type,
			"tenant_id": event.Principal.TenantID,
			"roles":     event.Principal.Roles,
			"method":    string(event.Principal.Method),
		}
	}
	
	if event.Error != "" {
		logEntry["error"] = event.Error
	}
	
	if event.RequestID != "" {
		logEntry["request_id"] = event.RequestID
	}
	
	if event.ClientIP != "" {
		logEntry["client_ip"] = event.ClientIP
	}
	
	if event.UserAgent != "" {
		logEntry["user_agent"] = event.UserAgent
	}
	
	if len(event.Context) > 0 {
		logEntry["context"] = event.Context
	}
	
	return f.encoder.Encode(logEntry)
}

// LogAuth logs an authentication event to file
func (f *FileAuditLogger) LogAuth(ctx context.Context, event *AuthEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	logEntry := map[string]interface{}{
		"type":      "auth",
		"timestamp": event.Timestamp.Format(time.RFC3339),
		"method":    string(event.Method),
		"result":    string(event.Result),
	}
	
	if event.Principal != nil {
		logEntry["principal"] = map[string]interface{}{
			"id":        event.Principal.ID,
			"type":      event.Principal.Type,
			"tenant_id": event.Principal.TenantID,
			"roles":     event.Principal.Roles,
		}
	}
	
	if event.Error != "" {
		logEntry["error"] = event.Error
	}
	
	if event.ClientIP != "" {
		logEntry["client_ip"] = event.ClientIP
	}
	
	if event.UserAgent != "" {
		logEntry["user_agent"] = event.UserAgent
	}
	
	return f.encoder.Encode(logEntry)
}

// Close closes the audit log file
func (f *FileAuditLogger) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

// SyslogAuditLogger implements AuditLogger using syslog (placeholder for Windows compatibility)
type SyslogAuditLogger struct {
	mu sync.Mutex
}

// NewSyslogAuditLogger creates a new syslog-based audit logger
func NewSyslogAuditLogger() (*SyslogAuditLogger, error) {
	// Note: syslog is not available on Windows, this is a placeholder implementation
	return &SyslogAuditLogger{}, nil
}

// LogAccess logs an access event to syslog (placeholder)
func (s *SyslogAuditLogger) LogAccess(ctx context.Context, event *AccessEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Placeholder implementation - in a real system this would use syslog
	logData := map[string]interface{}{
		"type":      "access",
		"timestamp": event.Timestamp.Format(time.RFC3339),
		"resource":  event.Resource,
		"action":    event.Action,
		"result":    string(event.Result),
	}
	
	if event.Principal != nil {
		logData["principal_id"] = event.Principal.ID
		logData["principal_type"] = event.Principal.Type
		logData["tenant_id"] = event.Principal.TenantID
		logData["auth_method"] = string(event.Principal.Method)
	}
	
	if event.Error != "" {
		logData["error"] = event.Error
	}
	
	if event.RequestID != "" {
		logData["request_id"] = event.RequestID
	}
	
	if event.ClientIP != "" {
		logData["client_ip"] = event.ClientIP
	}
	
	// In a real implementation, this would be sent to syslog
	_, err := json.Marshal(logData)
	return err
}

// LogAuth logs an authentication event to syslog (placeholder)
func (s *SyslogAuditLogger) LogAuth(ctx context.Context, event *AuthEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	logData := map[string]interface{}{
		"type":      "auth",
		"timestamp": event.Timestamp.Format(time.RFC3339),
		"method":    string(event.Method),
		"result":    string(event.Result),
	}
	
	if event.Principal != nil {
		logData["principal_id"] = event.Principal.ID
		logData["principal_type"] = event.Principal.Type
		logData["tenant_id"] = event.Principal.TenantID
	}
	
	if event.Error != "" {
		logData["error"] = event.Error
	}
	
	if event.ClientIP != "" {
		logData["client_ip"] = event.ClientIP
	}
	
	// In a real implementation, this would be sent to syslog
	_, err := json.Marshal(logData)
	return err
}

// Close closes the syslog connection (placeholder)
func (s *SyslogAuditLogger) Close() error {
	return nil
}

// MemoryAuditLogger implements AuditLogger using in-memory storage (for testing)
type MemoryAuditLogger struct {
	accessEvents []AccessEvent
	authEvents   []AuthEvent
	mu           sync.RWMutex
}

// NewMemoryAuditLogger creates a new in-memory audit logger
func NewMemoryAuditLogger() *MemoryAuditLogger {
	return &MemoryAuditLogger{
		accessEvents: make([]AccessEvent, 0),
		authEvents:   make([]AuthEvent, 0),
	}
}

// LogAccess logs an access event to memory
func (m *MemoryAuditLogger) LogAccess(ctx context.Context, event *AccessEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.accessEvents = append(m.accessEvents, *event)
	return nil
}

// LogAuth logs an authentication event to memory
func (m *MemoryAuditLogger) LogAuth(ctx context.Context, event *AuthEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.authEvents = append(m.authEvents, *event)
	return nil
}

// GetAccessEvents returns all access events (for testing)
func (m *MemoryAuditLogger) GetAccessEvents() []AccessEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events := make([]AccessEvent, len(m.accessEvents))
	copy(events, m.accessEvents)
	return events
}

// GetAuthEvents returns all auth events (for testing)
func (m *MemoryAuditLogger) GetAuthEvents() []AuthEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events := make([]AuthEvent, len(m.authEvents))
	copy(events, m.authEvents)
	return events
}

// Clear clears all events (for testing)
func (m *MemoryAuditLogger) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.accessEvents = m.accessEvents[:0]
	m.authEvents = m.authEvents[:0]
}