# ScyllaDB Migration System

This document describes the database schema migration system for the ScyllaDB state store in IPFS-Cluster.

## Overview

The migration system provides:
- **Schema versioning** - Track and manage database schema versions
- **Automatic migrations** - Apply schema changes automatically at startup
- **Compatibility checks** - Ensure schema versions are compatible with the application
- **Rollback support** - Limited rollback capabilities for emergency scenarios
- **Validation** - Comprehensive schema structure validation

## Architecture

### Core Components

1. **MigrationManager** (`migrations.go`)
   - Central coordinator for all migration operations
   - Manages migration registration, execution, and validation
   - Handles version comparison and compatibility checks

2. **Migration Scripts** (`migration_scripts.go`)
   - Contains all SQL migration scripts
   - Organized by version with up/down scripts
   - Includes validation and optimization scripts

3. **Migration Utilities** (`migration_utils.go`)
   - High-level utility functions for common operations
   - Configuration management for migration behavior
   - Result reporting and status tracking

4. **Migration CLI** (`migration_cli.go`)
   - Command-line interface for migration operations
   - Interactive and batch operation modes
   - Comprehensive reporting and diagnostics

### Schema Versioning

The system uses semantic versioning (MAJOR.MINOR.PATCH):
- **Current Version**: `1.1.0`
- **Minimum Supported**: `1.0.0`
- **Version Table**: `schema_version`

Version information is stored in the `schema_version` table:
```sql
CREATE TABLE schema_version (
    version text PRIMARY KEY,
    applied_at timestamp,
    comment text
);
```

## Migration Process

### Automatic Migration (Recommended)

The system can automatically apply migrations at startup:

```go
// Enable automatic migration in configuration
config := &MigrationConfig{
    AutoMigrate:      true,
    ValidateSchema:   true,
    MigrationTimeout: 5 * time.Minute,
}

// Apply migrations
result, err := EnsureSchemaReady(ctx, session, keyspace, config)
if err != nil {
    log.Fatalf("Migration failed: %v", err)
}
```

### Manual Migration

For production environments, you may prefer manual control:

```go
mm := NewMigrationManager(session, keyspace)

// Check current status
status, err := GetMigrationStatus(ctx, session, keyspace)
if err != nil {
    return err
}

if status.MigrationsNeeded {
    // Apply migrations manually
    err = mm.AutoMigrate(ctx)
    if err != nil {
        return fmt.Errorf("migration failed: %w", err)
    }
}
```

### CLI Operations

The migration CLI provides comprehensive management capabilities:

```bash
# Check migration status
migration-cli status

# Apply pending migrations
migration-cli migrate

# Validate schema structure
migration-cli validate

# View migration history
migration-cli history

# Generate detailed report
migration-cli report json > migration-report.json

# Health check
migration-cli health

# Get recommendations
migration-cli recommendations
```

## Schema Versions

### Version 1.0.0 (Initial Schema)

**Tables Created:**
- `pins_by_cid` - Main pin metadata storage
- `placements_by_cid` - Pin placement tracking
- `pins_by_peer` - Reverse index for peer queries
- `pin_ttl_queue` - TTL-based cleanup queue
- `op_dedup` - Operation deduplication
- `pin_stats` - Statistics counters
- `pin_events` - Audit log

**Key Features:**
- Optimized partitioning by multihash prefix
- Multi-datacenter replication support
- Efficient query patterns for trillion-scale data

### Version 1.1.0 (Enhanced Metadata)

**Schema Changes:**
- Added columns to `pins_by_cid`: `version`, `checksum`, `priority`
- Added columns to `placements_by_cid`: `in_progress`, `last_reconcile`
- Added columns to `pins_by_peer`: `assigned_at`, `completed_at`, `error_message`

**New Tables:**
- `partition_stats` - Load balancing metrics
- `performance_metrics` - Time-series performance data
- `batch_operations` - Batch operation tracking

**Performance Improvements:**
- Enhanced compression settings
- Optimized read repair configuration
- Better caching strategies

## Configuration

### Migration Configuration

```go
type MigrationConfig struct {
    AutoMigrate           bool          // Enable automatic migration
    MigrationTimeout      time.Duration // Timeout for migration operations
    ValidateSchema        bool          // Validate schema after migration
    AllowDowngrade        bool          // Allow downgrading (dangerous)
    BackupBeforeMigration bool          // Create backup before migration
}
```

### Environment Variables

The system supports environment-based configuration:

```bash
# Enable automatic migration
SCYLLADB_AUTO_MIGRATE=true

# Set migration timeout
SCYLLADB_MIGRATION_TIMEOUT=300s

# Enable schema validation
SCYLLADB_VALIDATE_SCHEMA=true
```

## Safety and Best Practices

### Pre-Migration Checklist

1. **Backup Data**: Always backup your data before major migrations
2. **Test Environment**: Test migrations in a staging environment first
3. **Maintenance Window**: Schedule migrations during maintenance windows
4. **Monitor Resources**: Ensure sufficient disk space and memory
5. **Review Changes**: Review migration scripts before applying

### Migration Safety Features

1. **Idempotent Operations**: All migrations can be run multiple times safely
2. **Compatibility Checks**: Automatic validation of version compatibility
3. **Rollback Scripts**: Limited rollback support for emergency scenarios
4. **Validation**: Comprehensive post-migration validation
5. **Error Handling**: Graceful handling of common migration errors

### Production Recommendations

1. **Manual Control**: Disable auto-migration in production
2. **Staged Rollout**: Apply migrations to one datacenter at a time
3. **Monitoring**: Monitor migration progress and system health
4. **Rollback Plan**: Have a tested rollback plan ready
5. **Communication**: Coordinate with operations team

## Troubleshooting

### Common Issues

#### Migration Timeout
```
Error: migration failed: context deadline exceeded
```
**Solution**: Increase migration timeout or run during low-traffic periods

#### Schema Validation Failed
```
Error: schema validation failed: missing table partition_stats
```
**Solution**: Check if migration completed successfully, re-run if needed

#### Version Compatibility Error
```
Error: schema version 2.0.0 is newer than supported version 1.1.0
```
**Solution**: Upgrade application before running migrations

#### Connection Issues
```
Error: failed to connect to ScyllaDB cluster
```
**Solution**: Verify connection settings and cluster health

### Diagnostic Commands

```bash
# Check migration status
migration-cli status

# Validate schema structure
migration-cli validate

# Generate diagnostic report
migration-cli report summary

# Check system health
migration-cli health

# Get recommendations
migration-cli recommendations
```

### Recovery Procedures

#### Incomplete Migration
1. Check migration status: `migration-cli status`
2. Validate current schema: `migration-cli validate`
3. Re-run migration: `migration-cli migrate`
4. Verify completion: `migration-cli validate`

#### Schema Corruption
1. Stop application services
2. Restore from backup if available
3. Re-run migrations from known good state
4. Validate schema integrity
5. Resume services

## Development

### Adding New Migrations

1. **Create Migration Script**:
   ```go
   func getV120MigrationScript() string {
       return `
       -- Migration from version 1.1.0 to 1.2.0
       ALTER TABLE ipfs_pins.pins_by_cid ADD new_column text;
       `
   }
   ```

2. **Register Migration**:
   ```go
   {
       Version:     "1.2.0",
       Description: "Add new functionality",
       UpScript:    getV120MigrationScript(),
       Validate:    mm.validateV120Schema,
   }
   ```

3. **Add Validation**:
   ```go
   func (mm *MigrationManager) validateV120Schema(session *gocql.Session) error {
       // Validate new schema features
       return nil
   }
   ```

4. **Update Constants**:
   ```go
   const CurrentSchemaVersion = "1.2.0"
   ```

### Testing Migrations

1. **Unit Tests**: Test migration logic and version comparison
2. **Integration Tests**: Test with real ScyllaDB instance
3. **Performance Tests**: Validate migration performance at scale
4. **Rollback Tests**: Test rollback procedures

### Migration Guidelines

1. **Backward Compatibility**: Ensure migrations don't break existing functionality
2. **Idempotency**: All operations should be safe to run multiple times
3. **Performance**: Consider impact on large datasets
4. **Documentation**: Document all schema changes thoroughly
5. **Testing**: Comprehensive testing in staging environment

## Monitoring and Metrics

### Migration Metrics

The system provides metrics for monitoring migration operations:

- `migration_duration_seconds` - Time taken for migrations
- `migration_success_total` - Count of successful migrations
- `migration_failure_total` - Count of failed migrations
- `schema_version_info` - Current schema version information

### Health Checks

Regular health checks ensure migration system integrity:

```go
// Perform health check
err := MigrationHealthCheck(ctx, session, keyspace)
if err != nil {
    log.Errorf("Migration system unhealthy: %v", err)
}
```

## Security Considerations

### Access Control

1. **Database Permissions**: Ensure migration user has necessary DDL permissions
2. **Schema Changes**: Restrict schema modification to authorized personnel
3. **Audit Logging**: Log all migration operations for security audit
4. **Backup Security**: Secure backup files and access credentials

### Production Security

1. **TLS Encryption**: Use TLS for all database connections
2. **Authentication**: Require strong authentication for migration operations
3. **Network Security**: Restrict network access to database cluster
4. **Monitoring**: Monitor for unauthorized schema changes

## Performance Considerations

### Large Scale Migrations

For trillion-scale deployments:

1. **Batch Processing**: Process migrations in batches to avoid timeouts
2. **Resource Monitoring**: Monitor CPU, memory, and disk usage
3. **Parallel Execution**: Use multiple connections for large operations
4. **Throttling**: Implement throttling to avoid overwhelming the cluster

### Optimization Strategies

1. **Off-Peak Execution**: Run migrations during low-traffic periods
2. **Resource Allocation**: Ensure sufficient cluster resources
3. **Compaction Management**: Manage compaction during migrations
4. **Monitoring**: Continuous monitoring of migration progress

## Future Enhancements

### Planned Features

1. **Automated Rollback**: Enhanced rollback capabilities
2. **Migration Scheduling**: Scheduled migration execution
3. **Multi-Tenant Support**: Tenant-specific migration management
4. **Advanced Validation**: More comprehensive schema validation
5. **Performance Optimization**: Further performance improvements

### Roadmap

- **v1.2.0**: Enhanced security features and audit logging
- **v1.3.0**: Multi-tenant migration support
- **v2.0.0**: Major architecture improvements and new features

## Support and Resources

### Documentation
- [ScyllaDB Documentation](https://docs.scylladb.com/)
- [IPFS-Cluster Documentation](https://cluster.ipfs.io/)
- [Migration Best Practices](./MIGRATION_BEST_PRACTICES.md)

### Community
- [IPFS-Cluster GitHub](https://github.com/ipfs-cluster/ipfs-cluster)
- [ScyllaDB Community](https://www.scylladb.com/community/)
- [IPFS Community](https://ipfs.io/community/)

### Support Channels
- GitHub Issues for bug reports and feature requests
- Community forums for general questions
- Professional support for enterprise deployments