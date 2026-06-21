package ipinfo

type options struct {
	useLookupRecordPool bool
}

type Option func(*options)

// WithLookupRecordPool enables the use of a pool for lookup records, which can improve performance by reusing memory for lookup results.
// But it requires that the record be freed with record.Free()
func WithLookupRecordPool() Option {
	return func(o *options) {
		o.useLookupRecordPool = true
	}
}
