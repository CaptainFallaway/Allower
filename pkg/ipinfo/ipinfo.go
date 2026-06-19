package ipinfo

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/oschwald/maxminddb-golang/v2"
)

type DB struct {
	dataset *managedDataset
	table   atomic.Pointer[maxminddb.Reader]
}

func New(token, storagePath string) *DB {
	return &DB{
		dataset: newManagedDataset(token, storagePath),
	}
}

// Sync synchronizes the local dataset with the remote source. It returns true if the dataset was updated, false if it was already up to date, and an error if the synchronization failed.
func (d *DB) Sync(ctx context.Context) (bool, error) {
	return d.dataset.Sync(ctx)
}

// Load loads the dataset into memory atomically. It should be called after Sync to ensure that the latest data is available. It returns an error if the loading process fails.
func (d *DB) Load() error {
	newReader, err := d.dataset.NewMmdbReader()
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	d.table.Store(newReader)

	return nil
}

func (d *DB) View(fn func(*maxminddb.Reader) error) error {
	readerPtr := d.table.Load()
	if readerPtr == nil {
		return fmt.Errorf("dataset not loaded")
	}
	return fn(readerPtr)
}

// Lookup looks up the given IP address in the loaded dataset and returns the corresponding record if found. It returns nil if the dataset has not been loaded.
func (d *DB) Lookup(addr netip.Addr) (*Record, error) {
	reader := d.table.Load()
	if reader == nil {
		return nil, fmt.Errorf("dataset not loaded")
	}

	result := reader.Lookup(addr)
	if result.Err() != nil {
		return nil, fmt.Errorf("failed to lookup IP address: %w", result.Err())
	} else if !result.Found() {
		return nil, nil
	}

	var record Record
	err := result.Decode(&record)
	if err != nil {
		return nil, fmt.Errorf("failed to decode record: %w", err)
	}

	return &record, nil
}
