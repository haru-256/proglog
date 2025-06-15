// Package server provides HTTP server implementation for the proglog service.
// It includes in-memory log storage and basic CRUD operations.
package server

import (
	"fmt"
	"sync"
)

// Log represents an in-memory log that stores records with sequential offsets.
// It provides thread-safe operations for appending and reading records.
type Log struct {
	mu      sync.Mutex
	records []Record
}

// NewLog creates and returns a new empty Log instance.
func NewLog() *Log {
	return &Log{}
}

// Append adds a new record to the log and returns its assigned offset.
// The offset is automatically set to the current length of the records slice.
// This operation is thread-safe.
func (c *Log) Append(record Record) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	record.Offset = uint64(len(c.records))
	c.records = append(c.records, record)
	return record.Offset, nil
}

// Read retrieves a record from the log at the specified offset.
// Returns ErrOffsetNotFound if the offset is out of bounds.
// This operation is thread-safe.
func (c *Log) Read(offset uint64) (Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if offset >= uint64(len(c.records)) {
		return Record{}, ErrOffsetNotFound
	}
	return c.records[offset], nil
}

// Record represents a single log entry with its value and offset.
type Record struct {
	Value  []byte `json:"value"`  // The actual data stored in the record
	Offset uint64 `json:"offset"` // The sequential position of the record in the log
}

// ErrOffsetNotFound is returned when attempting to read a record at an invalid offset.
var ErrOffsetNotFound = fmt.Errorf("offset not found")
