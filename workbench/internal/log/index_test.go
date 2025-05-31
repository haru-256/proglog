package log

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndex(t *testing.T) {
	f, err := os.CreateTemp(os.TempDir(), "index_test")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	c := Config{}
	c.Segment.MaxIndexBytes = 1024
	idx, err := newIndex(f, c)
	require.NoError(t, err)
	_, _, err = idx.Read(-1)
	require.Error(t, err, "expected error when reading from an empty index") // Expect an error when reading from an empty index
	require.Equal(t, f.Name(), idx.Name(), "expected index name to match file name")

	entries := []struct {
		Off uint32
		Pos uint64
	}{
		{Off: 0, Pos: 0},
		{Off: 1, Pos: 10},
	}

	for _, want := range entries {
		err = idx.Write(want.Off, want.Pos)
		require.NoError(t, err)

		_, pos, err := idx.Read(int64(want.Off))
		require.NoError(t, err)
		require.Equal(t, want.Pos, pos, "expected position %d, got %d", want.Pos, pos)
	}

	// Test when reading an out-of-bounds index
	_, _, err = idx.Read(int64(len(entries)))
	require.Error(t, err, "expected error when reading out of bounds index")
	_ = idx.Close()

	// Reopen the index file and verify the last entry
	f, err = os.OpenFile(f.Name(), os.O_RDWR, 0600)
	idx, err = newIndex(f, c)
	require.NoError(t, err)
	off, pos, err := idx.Read(-1)
	require.NoError(t, err)
	require.Equal(t, entries[1].Off, off, "expected last offset to be 1")
	require.Equal(t, entries[1].Pos, pos, "expected last position to be 10")
}
