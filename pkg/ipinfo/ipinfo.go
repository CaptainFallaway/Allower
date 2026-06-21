package ipinfo

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/oschwald/maxminddb-golang/v2"
)

var recordPool = &sync.Pool{
	New: func() any {
		return &Record{}
	},
}

type Dataset struct {
	options options

	dataset *managedDataset
	table   atomic.Pointer[maxminddb.Reader]
}

func New(token, storageDir string, opts ...Option) *Dataset {
	options := new(options)
	for _, opt := range opts {
		opt(options)
	}

	return &Dataset{
		options: *options,
		dataset: newManagedDataset(token, storageDir),
	}
}

// Sync synchronizes the local dataset with the remote source. It returns true if the dataset was updated, false if it was already up to date, and an error if the synchronization failed.
func (d *Dataset) Sync(ctx context.Context) (bool, error) {
	return d.dataset.Sync(ctx)
}

// Load loads the dataset into memory atomically. It should be called after Sync to ensure that the latest data is available. It returns an error if the loading process fails.
func (d *Dataset) Load() error {
	newReader, err := d.dataset.newMmdbReader()
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	d.table.Store(newReader)

	return nil
}

func (d *Dataset) View(fn func(*maxminddb.Reader) error) error {
	readerPtr := d.table.Load()
	if readerPtr == nil {
		return fmt.Errorf("dataset not loaded")
	}
	return fn(readerPtr)
}

// Lookup looks up the given IP address in the loaded dataset and returns the corresponding record if found. It returns nil if the dataset has not been loaded.
func (d *Dataset) Lookup(addr netip.Addr) (*Record, error) {
	if !addr.IsValid() ||
		addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsUnspecified() ||
		addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() {
		return nil, ErrAddrIsPrivate
	}

	reader := d.table.Load()
	if reader == nil {
		return nil, fmt.Errorf("dataset not loaded")
	}

	result := reader.Lookup(addr)
	if result.Err() != nil {
		return nil, fmt.Errorf("failed to lookup IP address: %w", result.Err())
	} else if !result.Found() {
		return nil, ErrNotFound
	}

	var record *Record

	if d.options.useLookupRecordPool {
		record = recordPool.Get().(*Record)
		record.pool = recordPool
	} else {
		record = new(Record)
	}

	err := result.Decode(&record)
	if err != nil {
		return nil, fmt.Errorf("failed to decode record: %w", err)
	}

	return record, nil
}
