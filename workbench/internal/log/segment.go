package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	api "github.com/haru-256/proglog/api/v1"
	"google.golang.org/protobuf/proto"
)

type segment struct {
	store                  *store
	index                  *index
	baseOffset, nextOffset uint64
	config                 Config
}

func newSegment(dir string, baseOffset uint64, c Config) (*segment, error) {
	s := &segment{
		baseOffset: baseOffset,
		config:     c,
	}
	storeFile, err := os.OpenFile(
		filepath.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".store")),
		os.O_RDWR|os.O_CREATE|os.O_APPEND,
		0600,
	)
	if err != nil {
		return nil, err
	}
	if s.store, err = newStore(storeFile); err != nil {
		return nil, err
	}
	indexFile, err := os.OpenFile(
		filepath.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".index")),
		os.O_RDWR|os.O_CREATE, // O_APPEND is not used because the index file requires precise control over read and write offsets, which is incompatible with appending all writes to the end of the file.
		0600,
	)
	if err != nil {
		return nil, err
	}
	if s.index, err = newIndex(indexFile, c); err != nil {
		return nil, err
	}
	if off, _, err := s.index.Read(-1); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("read last index entry: %w", err)
		}
		s.nextOffset = baseOffset
	} else {
		s.nextOffset = baseOffset + uint64(off) + 1
	}
	return s, nil
}

// Append appends a record to the segment.
// It returns the absolute offset of the record in the log and an error if any.
func (s *segment) Append(record *api.Record) (offset uint64, err error) {
	cur := s.nextOffset
	record.Offset = cur
	p, err := proto.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("marshal record: %w", err)
	}
	_, pos, err := s.store.Append(p)
	if err != nil {
		return 0, fmt.Errorf("append record: %w", err)
	}

	// index offset is the relative offset from the base offset of the segment
	if err := s.index.Write(uint32(record.Offset-s.baseOffset), pos); err != nil {
		return 0, fmt.Errorf("write index: %w", err)
	}
	s.nextOffset++ // increment nextOffset for the next record
	return cur, nil
}

// Read reads a record at the given offset from the segment.
// off is the absolute offset in the log, not relative to the segment.
func (s *segment) Read(off uint64) (*api.Record, error) {
	_, pos, err := s.index.Read(int64(off - s.baseOffset))
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	p, err := s.store.Read(pos)
	if err != nil {
		return nil, fmt.Errorf("read store: %w", err)
	}
	record := &api.Record{}
	err = proto.Unmarshal(p, record)
	return record, err
}

func (s *segment) IsMaxed() bool {
	return s.store.size >= s.config.Segment.MaxStoreBytes ||
		s.index.size >= s.config.Segment.MaxIndexBytes ||
		s.index.isMaxed()
}

func (s *segment) Remove() error {
	if err := s.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	if err := os.Remove(s.store.Name()); err != nil {
		return fmt.Errorf("remove store file: %w", err)
	}
	if err := os.Remove(s.index.Name()); err != nil {
		return fmt.Errorf("remove index file: %w", err)
	}
	return nil
}

func (s *segment) Close() error {
	if err := s.store.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	if err := s.index.Close(); err != nil {
		return fmt.Errorf("close index: %w", err)
	}
	return nil
}
