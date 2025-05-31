package log

import (
	"fmt"
	"io"
	"os"

	"github.com/tysonmote/gommap"
)

const (
	// offWidth is the number of bytes used to store the record offset in the index.
	// This 4-byte field stores the relative offset of a record within a segment.
	offWidth uint64 = 4

	// posWidth is the number of bytes used to store the position in the store file.
	// This 8-byte field stores the absolute position where the record is stored.
	posWidth uint64 = 8

	// entWidth is the total width of each index entry in bytes.
	// Each entry consists of an offset (4 bytes) and a position (8 bytes).
	entWidth uint64 = offWidth + posWidth
)

// index represents a memory-mapped index file that stores offset-position pairs.
// It maintains efficient lookups for record positions in the corresponding store file.
// The index file contains pairs of record offsets and their positions in the store,
// with the next write position managed by the size field.
type index struct {
	file *os.File
	mmap gommap.MMap
	size uint64 // size of the index in bytes
}

// newIndex creates a new index instance from an existing file.
// It initializes the index with the file's current size and sets up memory mapping
// for efficient random access. The file is truncated to the maximum index size
// specified in the configuration to ensure consistent memory mapping.
func newIndex(f *os.File, c Config) (*index, error) {
	idx := &index{
		file: f,
	}
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	idx.size = uint64(fi.Size())
	// Ensure the file is large enough to hold the index entries.
	if err = os.Truncate(f.Name(), int64(c.Segment.MaxIndexBytes)); err != nil {
		return nil, err
	}

	if idx.mmap, err = gommap.Map(idx.file.Fd(), gommap.PROT_READ|gommap.PROT_WRITE, gommap.MAP_SHARED); err != nil {
		return nil, err
	}

	return idx, nil
}

// Close synchronizes the memory-mapped data to disk and closes the index file.
// It ensures all pending writes are flushed and truncates the file to the actual
// data size before closing. This method should be called before the index is discarded.
func (i *index) Close() error {
	if err := i.mmap.Sync(gommap.MS_SYNC); err != nil {
		return err
	}
	if err := i.file.Sync(); err != nil {
		return err
	}
	if err := i.file.Truncate(int64(i.size)); err != nil {
		return err
	}
	return i.file.Close()
}

// Read reads an entry from the index.
// in is the index offset to read, or -1 to read the last entry.
// out is the offset of the record in the store file, and pos is the position in the index file.
func (i *index) Read(in int64) (out uint32, pos uint64, err error) {
	if i.size == 0 {
		return 0, 0, io.EOF
	}
	if in < -1 {
		return 0, 0, fmt.Errorf("index: invalid index %d", in)
	}
	var indexOff uint32 // indexOff is the offset of index
	var indexPos uint64 // indexPos is the position in the index file
	if in == -1 {
		indexOff = uint32((i.size / entWidth) - 1)
	} else {
		indexOff = uint32(in)
	}
	indexPos = uint64(indexOff) * entWidth
	if i.size < indexPos+entWidth {
		return 0, 0, io.EOF
	}
	out = enc.Uint32(i.mmap[indexPos : indexPos+offWidth])
	pos = enc.Uint64(i.mmap[indexPos+offWidth : indexPos+entWidth])
	return out, pos, nil
}

// Write adds a new entry to the index.
// off is the offset of the record in the store file, and pos is the position in the index file.
// It returns io.EOF if the index is full and cannot accommodate more entries.
func (i *index) Write(off uint32, pos uint64) error {
	if i.isMaxed() {
		return io.EOF
	}
	// recordのoffsetを追加
	enc.PutUint32(i.mmap[i.size:i.size+offWidth], off)
	// recordの内容が書き込まれたstoreファイルの位置を追加
	enc.PutUint64(i.mmap[i.size+offWidth:i.size+entWidth], pos)
	i.size += entWidth
	return nil
}

// isMaxed checks if the index has reached its maximum capacity.
// It returns true if adding another entry would exceed the memory-mapped region size.
// This is used to determine when a new segment should be created.
func (i *index) isMaxed() bool {
	// 次のエントリを追加するために必要なサイズが、現在のmmapのサイズより小さいかどうかを確認
	return uint64(len(i.mmap)) < i.size+entWidth
}

// Name returns the name of the index file.
// This method provides access to the underlying file name for debugging
// and logging purposes.
func (i *index) Name() string {
	return i.file.Name()
}
