package log

import (
	"fmt"
	"io"
	"os"

	"github.com/tysonmote/gommap"
)

const (
	offWidth uint64 = 4
	posWidth uint64 = 8
	entWidth uint64 = offWidth + posWidth
)

// indexのファイルには、recordのオフセットと、recordの内容が書き込まれたstoreファイルの位置のペアが格納される
// index自体の次の書き込み位置は、indexのサイズで管理される
type index struct {
	file *os.File
	mmap gommap.MMap
	size uint64 // size of the index in bytes
}

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

func (i *index) isMaxed() bool {
	// 次のエントリを追加するために必要なサイズが、現在のmmapのサイズより小さいかどうかを確認
	return uint64(len(i.mmap)) < i.size+entWidth
}

func (i *index) Name() string {
	return i.file.Name()
}
