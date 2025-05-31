package log

// Config holds configuration parameters for the log and its segments.
// It contains settings that control the maximum sizes and initial offsets
// for both store and index files within segments.
type Config struct {
	// Segment contains configuration parameters specific to log segments.
	Segment struct {
		// MaxStoreBytes defines the maximum size in bytes for a store file.
		// When a store file reaches this size, a new segment is created.
		MaxStoreBytes uint64

		// MaxIndexBytes defines the maximum size in bytes for an index file.
		// When an index file reaches this size, a new segment is created.
		MaxIndexBytes uint64

		// InitialOffset sets the starting offset for the first segment
		// when creating a new log from an empty directory.
		InitialOffset uint64
	}
}
