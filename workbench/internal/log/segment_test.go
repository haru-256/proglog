package log

import (
	"io"
	"os"
	"testing"

	api "github.com/haru-256/proglog/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSegment(t *testing.T) {
	dir, err := os.MkdirTemp("", "segment_test")
	require.NoError(t, err, "expected no error when creating temporary directory")
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	want := &api.Record{Value: []byte("hello world")}

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = entWidth * 3
	baseOffset := uint64(16)

	s, err := newSegment(dir, baseOffset, c)
	require.NoError(t, err)
	require.Equal(t, baseOffset, s.nextOffset, "expected next offset to be 16")
	require.False(t, s.IsMaxed(), "expected segment to not be maxed")

	for i := uint64(0); i < 3; i++ {
		off, err := s.Append(want)
		require.NoError(t, err, "expected no error when appending record")
		require.Equal(t, baseOffset+i, off, "expected offset to match next offset")

		got, err := s.Read(off)
		require.NoError(t, err, "expected no error when reading record")
		require.Equal(t, want.Value, got.Value, "expected record value to match")
	}

	_, err = s.Append(want)
	require.ErrorIs(t, err, io.EOF, "expected io.EOF error when appending to maxed segment")

	require.True(t, s.IsMaxed(), "expected segment to be maxed")
	require.NoError(t, s.Close(), "expected no error when closing segment")

	p, _ := proto.Marshal(want)
	c.Segment.MaxStoreBytes = uint64(len(p)+lenWidth) * 4 // (byte length + record length) x 4
	c.Segment.MaxIndexBytes = 1024
	// Recreate the segment with a new config
	s, err = newSegment(dir, baseOffset, c)
	require.NoError(t, err, "expected no error when creating new segment")
	// Check if the segment is maxed
	require.True(t, s.IsMaxed(), "expected segment to be maxed after recreation")

	require.NoError(t, s.Remove(), "expected no error when removing segment")

	s, err = newSegment(dir, baseOffset, c)
	require.NoError(t, err, "expected no error when creating new segment after removal")
	require.False(t, s.IsMaxed(), "expected segment to not be maxed after removal")
	require.NoError(t, s.Close(), "expected no error when closing segment after removal")
}
