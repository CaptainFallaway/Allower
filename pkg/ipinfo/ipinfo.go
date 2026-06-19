package ipinfo

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/gaissmai/bart"
)

type DB struct {
	dataset *managedDataset
	table   atomic.Pointer[bart.Fast[*Record]]
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
	reader, err := d.dataset.NewScanner()
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}
	defer reader.Close()

	newTable := new(bart.Fast[*Record])

	for prefix, record := range reader.All() {
		newTable.Insert(prefix, record)
	}

	d.table.Store(newTable)

	return reader.Err()
}

// Lookup looks up the given IP address in the loaded dataset and returns the corresponding record if found. It returns nil and false if the dataset has not been loaded.
func (d *DB) Lookup(addr netip.Addr) (*Record, bool) {
	table := d.table.Load()
	if table == nil {
		return nil, false
	}
	return table.Lookup(addr)
}
