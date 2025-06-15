package log

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	api "github.com/haru-256/proglog/api/v1"
)

// Log represents a distributed log composed of multiple segments.
// It provides thread-safe operations for appending and reading records.
type Log struct {
	mu sync.RWMutex

	Dir    string
	Config Config

	activeSegment *segment
	segments      []*segment
}

// NewLog creates a new Log instance with the specified directory and configuration.
// It initializes the log by setting up the directory and creating the initial segment if necessary.
// If the segment's maximum store or index bytes are not set, they default to 1024 bytes.
func NewLog(dir string, c Config) (*Log, error) {
	if c.Segment.MaxStoreBytes == 0 {
		c.Segment.MaxStoreBytes = 1024
	}
	if c.Segment.MaxIndexBytes == 0 {
		c.Segment.MaxIndexBytes = 1024
	}
	l := &Log{
		Dir:    dir,
		Config: c,
	}
	return l, l.setup()
}

// setup initializes the log by scanning the directory for existing segments
// and creating the initial segment if none exist. It reads all segment files,
// extracts their base offsets, sorts them, and recreates the segments in order.
func (l *Log) setup() error {
	files, err := os.ReadDir(l.Dir)
	if err != nil {
		return err
	}
	var baseOffsets []uint64
	for _, file := range files {
		// file.Name() は "{baseOffset}.store" または "{baseOffset}.index" の形式であることを想定
		offStr := strings.TrimSuffix(file.Name(), path.Ext(file.Name()))
		off, err := strconv.ParseUint(offStr, 10, 64)
		if err != nil {
			return fmt.Errorf("parse base offset %q: %w", offStr, err)
		}
		baseOffsets = append(baseOffsets, off)
	}
	slices.SortFunc(baseOffsets, func(i, j uint64) int {
		return cmp.Compare(i, j)
	})
	for i := 0; i < len(baseOffsets); i++ {
		if err = l.newSegment(baseOffsets[i]); err != nil {
			return err
		}
		// baseOffsets は、インデックスとストアの二つの重複を含んでいるので、重複しているものをスキップする
		// 例: 0.store, 0.index, 1.store, 1.index の場合、0.store と 0.index は同じ baseOffset を持つので、次のループに進む
		i++
	}
	// l.Dirにsegmentが存在しない場合、初期オフセットを設定して新しいセグメントを作成する
	if l.segments == nil {
		if err = l.newSegment(l.Config.Segment.InitialOffset); err != nil {
			return err
		}
	}

	return nil
}

// newSegment creates a new segment with the specified offset and adds it to the log.
// It initializes the segment with the provided directory and configuration.
func (l *Log) newSegment(off uint64) error {
	s, err := newSegment(l.Dir, off, l.Config)
	if err != nil {
		return err
	}
	l.segments = append(l.segments, s)
	l.activeSegment = s
	return nil
}

// Append adds a new record to the log and returns its offset.
// It automatically creates a new segment if the current active segment is full.
// The operation is thread-safe and ensures records are appended in order.
func (l *Log) Append(record *api.Record) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	highestOffset, err := l.highestOffset()
	if err != nil {
		return 0, err
	}

	if l.activeSegment.IsMaxed() {
		if err := l.newSegment(highestOffset + 1); err != nil {
			return 0, err
		}
	}

	offset, err := l.activeSegment.Append(record)
	if err != nil {
		return 0, err
	}
	return offset, nil
}

// Read reads the record stored at the given offset.
// It searches through all segments to find the one containing the offset,
// then delegates to that segment's Read method. The operation is thread-safe
// with read lock protection.
func (l *Log) Read(off uint64) (*api.Record, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var s *segment
	for _, segment := range l.segments {
		if segment.baseOffset <= off && off < segment.nextOffset {
			s = segment
			break
		}
	}
	if s == nil || s.nextOffset <= off {
		return nil, api.ErrOffsetOutOfRange{Offset: off}
	}
	return s.Read(off)
}

// Close closes the log and all its segments.
// It ensures that all data is flushed to disk and file handles are released.
// This method should be called when the log is no longer needed.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, segment := range l.segments {
		if err := segment.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Remove closes the log and removes all segment files from disk.
// This operation is irreversible and should be used with caution.
// It ensures proper cleanup by closing all segments before removing files.
func (l *Log) Remove() error {
	if err := l.Close(); err != nil {
		return err
	}
	return os.RemoveAll(l.Dir)
}

// Reset removes all data from the log and recreates it with a fresh state.
// This is useful for testing or when a complete reset is needed.
// It removes all existing data and reinitializes the log.
func (l *Log) Reset() error {
	if err := l.Remove(); err != nil {
		return err
	}
	return l.setup()
}

// LowestOffset returns the lowest offset available in the log.
// This corresponds to the base offset of the first segment.
// The operation is thread-safe with read lock protection.
func (l *Log) LowestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.segments) == 0 {
		return 0, fmt.Errorf("log contains no segments from which to determine the lowest offset")
	}
	return l.segments[0].baseOffset, nil
}

// HighestOffset returns the highest offset available in the log.
// This corresponds to the last written record's offset.
// The operation is thread-safe with read lock protection.
func (l *Log) HighestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.highestOffset()
}

// highestOffset is an internal method that returns the highest offset
// without acquiring locks. It calculates the offset based on the
// nextOffset of the last segment, returning 0 if no records exist.
func (l *Log) highestOffset() (uint64, error) {
	if len(l.segments) == 0 {
		return 0, fmt.Errorf("log contains no segments from which to determine the highest offset")
	}
	off := l.segments[len(l.segments)-1].nextOffset
	if off == 0 {
		return 0, nil
	}
	return off - 1, nil
}

// Truncate removes all segments with offsets less than or equal to the specified lowest offset.
// It ensures that the log retains only the segments with offsets greater than the specified value.
func (l *Log) Truncate(lowest uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var segments []*segment
	for _, s := range l.segments {
		if s.nextOffset <= lowest+1 {
			if err := s.Remove(); err != nil {
				return err
			}
			continue
		}
		segments = append(segments, s)
	}
	l.segments = segments
	return nil
}

// Reader returns an io.Reader that reads from all segments in the log sequentially.
// It creates a MultiReader that concatenates all segment stores, starting from the
// beginning of each segment. The returned reader allows reading the entire log
// contents as a continuous stream. This method is safe for concurrent use as it
// acquires a read lock during execution.
func (l *Log) Reader() io.Reader {
	l.mu.RLock()
	defer l.mu.RUnlock()
	readers := make([]io.Reader, len(l.segments))
	for i, segment := range l.segments {
		readers[i] = &originalReader{
			store: segment.store,
			off:   int64(0), // Start reading from the beginning of each segment's store
		}
	}
	return io.MultiReader(readers...)
}

// originalReader wraps a store with an offset position for reading log entries.
// It maintains the current read position (off) within the underlying store,
// allowing for sequential reading operations starting from a specific offset.
type originalReader struct {
	*store
	off int64
}

// ReadAt reads data into p starting from the current offset in the store.
func (o *originalReader) Read(p []byte) (int, error) {
	n, err := o.ReadAt(p, o.off)
	o.off += int64(n) // Update offset after reading
	return n, err
}
