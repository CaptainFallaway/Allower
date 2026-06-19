package ipinfo

import (
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"iter"
	"net/netip"
	"os"
)

const csvFieldsPerRecord = 8

// datasetScanner is responsible for reading the IP dataset from a gzipped CSV file.
// It's single use and should be closed after use to release resources.
type datasetScanner struct {
	file      *os.File
	gzReader  *gzip.Reader
	csvReader *csv.Reader

	// error encountered during iteration
	err error
}

func newDatasetScanner(path string) (*datasetScanner, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}

	csvReader := csv.NewReader(gzReader)
	csvReader.FieldsPerRecord = csvFieldsPerRecord
	csvReader.TrimLeadingSpace = true
	csvReader.ReuseRecord = true

	return &datasetScanner{
		file:      file,
		gzReader:  gzReader,
		csvReader: csvReader,
	}, nil
}

func (dr *datasetScanner) All() iter.Seq2[netip.Prefix, *Record] {
	return func(yield func(netip.Prefix, *Record) bool) {
		// Skip Header
		if _, err := dr.csvReader.Read(); err != nil {
			err = fmt.Errorf("failed initializing iteration: %w", err)
			return
		}

		count := 0

		for {
			fields, err := dr.csvReader.Read()
			if err != nil {
				err = fmt.Errorf("failed reading record %d: %w", count, err)
				return
			}

			prefix, err := netip.ParsePrefix(fields[0])
			if err != nil {
				err = fmt.Errorf("failed parsing prefix in record %d: %w", count, err)
				return
			}

			// Number of fields is already validated by csvReader, so we can safely index them.
			record := &Record{
				Country:       fields[1],
				CountryCode:   fields[2],
				Continent:     fields[3],
				ContinentCode: fields[4],
				AsNumber:      fields[5],
				AsName:        fields[6],
				AsDomain:      fields[7],
			}

			if !yield(prefix, record) {
				return
			}

			count++
		}
	}
}

func (dr *datasetScanner) Err() error {
	return dr.err
}

func (dr *datasetScanner) Close() error {
	return errors.Join(dr.file.Close(), dr.gzReader.Close())
}
