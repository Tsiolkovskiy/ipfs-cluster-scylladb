# Marshal/Unmarshal Implementation for ScyllaDB State Store

## Overview

This document describes the implementation of Marshal/Unmarshal functionality for the ScyllaDB state store, which allows exporting and importing the entire state of pins for backup, migration, and disaster recovery purposes.

## Implementation Details

### Task 8.2 Requirements

The implementation fulfills the following requirements from task 8.2:

1. **Marshal() method for state export** - Exports the entire pin state to a writer
2. **Unmarshal() method for state import** - Imports pin state from a reader  
3. **Migrate() method for updating old formats** - Handles migration from legacy formats

### Data Format

The implementation uses a hybrid approach for maximum compatibility:

#### Export Format (Marshal)

1. **Header**: JSON-encoded metadata (optional for backward compatibility)
   ```json
   {
     "version": 1,
     "timestamp": "2024-01-01T00:00:00Z",
     "keyspace": "ipfs_pins"
   }
   ```

2. **Pin Data**: Length-prefixed protobuf format (compatible with dsstate)
   ```
   <length>\n
   <protobuf_data>
   \n
   ```

#### Import Format (Unmarshal)

The Unmarshal method supports multiple formats:

1. **Current format**: JSON header + length-prefixed protobuf pins
2. **Legacy format**: Length-prefixed protobuf pins only (no header)
3. **Migration support**: Automatic detection and conversion

### Key Features

#### 1. Backward Compatibility

- **Legacy format support**: Can import data exported by older versions
- **Header optional**: Works with or without JSON header
- **Protobuf consistency**: Uses same serialization as existing state stores

#### 2. Performance Optimizations

- **Batched operations**: Uses ScyllaBatchingState for efficient bulk imports
- **Streaming**: Processes large datasets without loading everything into memory
- **Progress logging**: Reports progress for large operations

#### 3. Error Handling

- **Graceful degradation**: Continues processing even if individual pins fail
- **Detailed logging**: Logs skipped pins with reasons
- **Transaction safety**: Uses batching to ensure consistency

#### 4. Migration Support

The Migrate() method supports multiple legacy formats:

- **Current format**: Direct unmarshal
- **Legacy length-prefixed**: Simple protobuf format
- **DSState format**: Placeholder for msgpack format (simplified to legacy for now)

### API Methods

#### Marshal(w io.Writer) error

Exports the entire state to a writer.

**Process:**
1. Write JSON header with version and metadata
2. Stream all pins from ScyllaDB
3. Serialize each pin using protobuf
4. Write in length-prefixed format
5. Log progress for large exports

**Example usage:**
```go
var buf bytes.Buffer
err := state.Marshal(&buf)
if err != nil {
    log.Fatal(err)
}
// buf now contains the exported state
```

#### Unmarshal(r io.Reader) error

Imports state from a reader.

**Process:**
1. Try to read JSON header (optional)
2. Validate version compatibility if header present
3. Read pins in length-prefixed format
4. Use batching for efficient bulk import
5. Commit in configurable batch sizes

**Example usage:**
```go
file, err := os.Open("state_backup.dat")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

err = state.Unmarshal(file)
if err != nil {
    log.Fatal(err)
}
```

#### Migrate(ctx context.Context, r io.Reader) error

Migrates state from older formats.

**Process:**
1. Detect format by reading initial bytes
2. Try current format first
3. Fall back to legacy formats
4. Convert and import using appropriate method

**Example usage:**
```go
oldStateFile, err := os.Open("old_state.dat")
if err != nil {
    log.Fatal(err)
}
defer oldStateFile.Close()

err = state.Migrate(context.Background(), oldStateFile)
if err != nil {
    log.Fatal(err)
}
```

### BatchingState Support

The ScyllaBatchingState also implements Marshal/Unmarshal:

- **Marshal**: Commits pending operations before delegating to underlying state
- **Unmarshal**: Clears pending batch before delegating to underlying state
- **Migrate**: Clears pending batch before delegating to underlying state

### Configuration

The implementation respects the following configuration options:

- **BatchSize**: Number of pins to process in each batch during import
- **BatchTimeout**: Maximum time to wait before committing a batch
- **PageSize**: Number of pins to fetch in each query during export

### Error Scenarios

#### Export Errors

- **Connection failures**: Logged and retried with graceful degradation
- **Serialization errors**: Individual pins skipped, operation continues
- **Write errors**: Operation fails immediately

#### Import Errors

- **Format errors**: Attempts multiple format parsers
- **Invalid pins**: Skipped with warning, operation continues
- **Batch failures**: Entire batch retried, then individual pins if needed

#### Migration Errors

- **Unknown format**: Returns error after trying all known formats
- **Partial success**: Logs successful count, returns last error

### Performance Characteristics

#### Export Performance

- **Memory usage**: Constant (streaming approach)
- **Time complexity**: O(n) where n is number of pins
- **Disk I/O**: Sequential writes, optimal for large datasets

#### Import Performance

- **Memory usage**: O(batch_size)
- **Time complexity**: O(n) with batching optimizations
- **Database I/O**: Batched writes for optimal ScyllaDB performance

### Testing

The implementation includes comprehensive tests:

- **Unit tests**: Data structure serialization/deserialization
- **Integration tests**: Full export/import cycles
- **Benchmark tests**: Performance measurement
- **Migration tests**: Legacy format compatibility

### Future Enhancements

Potential improvements for future versions:

1. **Compression**: Add optional compression for large exports
2. **Encryption**: Support encrypted exports for sensitive data
3. **Incremental exports**: Export only changes since last backup
4. **Parallel processing**: Multi-threaded import/export for very large datasets
5. **Checksums**: Data integrity verification
6. **Resume capability**: Resume interrupted operations

### Compatibility Matrix

| Source Format | Destination | Supported | Method |
|---------------|-------------|-----------|---------|
| ScyllaDB v1   | ScyllaDB v1 | ✅ | Marshal/Unmarshal |
| Legacy format | ScyllaDB v1 | ✅ | Migrate |
| DSState       | ScyllaDB v1 | ⚠️ | Migrate (simplified) |
| ScyllaDB v1   | DSState     | ✅ | Marshal (compatible) |

### Usage Examples

#### Backup and Restore

```go
// Backup
backupFile, err := os.Create("cluster_backup.dat")
if err != nil {
    log.Fatal(err)
}
defer backupFile.Close()

err = state.Marshal(backupFile)
if err != nil {
    log.Fatal(err)
}

// Restore
restoreFile, err := os.Open("cluster_backup.dat")
if err != nil {
    log.Fatal(err)
}
defer restoreFile.Close()

err = state.Unmarshal(restoreFile)
if err != nil {
    log.Fatal(err)
}
```

#### Migration from DSState

```go
dsStateFile, err := os.Open("dsstate_export.dat")
if err != nil {
    log.Fatal(err)
}
defer dsStateFile.Close()

err = scyllaState.Migrate(context.Background(), dsStateFile)
if err != nil {
    log.Fatal(err)
}
```

This implementation provides a robust, performant, and compatible solution for state export, import, and migration in the ScyllaDB state store.