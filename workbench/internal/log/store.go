package log

import (
	"bufio"
	"encoding/binary"
	"os"
	"sync"
)

var (
	// enc is the binary encoding used for writing and reading numeric values
	// in the store files. BigEndian ensures consistent byte ordering across platforms.
	enc = binary.BigEndian
)

const (
	// lenWidth is the number of bytes used to store the length of each record.
	// This 8-byte prefix allows the store to know how many bytes to read for each record.
	lenWidth = 8
)

// store represents a file-based storage for log records with buffered writes.
// It maintains the size of the file and provides thread-safe operations for
// appending and reading records. Each record is prefixed with its length.
type store struct {
	*os.File
	mu  sync.Mutex
	buf *bufio.Writer
	// size is the total size of the file in bytes, including the length of each record.
	size uint64
}

// newStore creates a new store instance from an existing file.
// It initializes the store with the file's current size and sets up
// a buffered writer for efficient writes. The function preserves
// any existing data in the file.
func newStore(f *os.File) (*store, error) {
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	size := uint64(fi.Size())
	return &store{
		File: f,
		size: size,
		buf:  bufio.NewWriter(f),
	}, nil
}

// Append appends a log record to the store.
// It writes the record length followed by the record data to the buffered writer.
// Returns the number of bytes written (including length prefix), the position
// where the record was written, and any error that occurred.
// The operation is thread-safe and updates the store's size tracker.
func (s *store) Append(p []byte) (n uint64, pos uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pos = s.size
	if err := binary.Write(s.buf, enc, uint64(len(p))); err != nil {
		return 0, 0, err
	}
	w, err := s.buf.Write(p)
	if err != nil {
		return 0, 0, err
	}
	w += lenWidth // include the length of the record
	s.size += uint64(w)

	return uint64(w), pos, nil
}

// Read reads a log record from the specified position in the store.
// It first reads the 8-byte length prefix to determine the record size,
// then reads the actual record data. The operation is thread-safe and
// flushes any buffered writes before reading to ensure data consistency.
func (s *store) Read(pos uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return nil, err
	}

	size := make([]byte, lenWidth)
	if _, err := s.File.ReadAt(size, int64(pos)); err != nil {
		return nil, err
	}

	b := make([]byte, enc.Uint64(size))
	if _, err := s.File.ReadAt(b, int64(pos+lenWidth)); err != nil {
		return nil, err
	}
	return b, nil
}

// ReadAt reads data into p starting from the specified offset in the store file.
// This method provides direct access to the underlying file data and is thread-safe.
// It flushes any buffered writes before reading to ensure data consistency.
// This is typically used for low-level access when the record format is known.
func (s *store) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return 0, err
	}

	return s.File.ReadAt(p, off)
}

// Close closes the store, flushing any buffered data to disk.
func (s *store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return err
	}
	return s.File.Close()
}
