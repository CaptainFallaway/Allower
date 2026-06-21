package ipinfo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/goccy/go-json" // Just because it's fun
	"github.com/oschwald/maxminddb-golang/v2"
)

const datasetName = "ipinfo_lite.mmdb"
const datasetDownloadURL = "https://ipinfo.io/data/" + datasetName
const datasetChecksumURL = "https://ipinfo.io/data/" + datasetName + "/checksums"

// managedDataset manages the local copy of the IP geolocation dataset.
// It has methods to check for updates, download the dataset, and create a reader for it.
type managedDataset struct {
	datasetPath string
	token       string
	hasher      hash.Hash

	client *http.Client
}

func newManagedDataset(token, storageDir string) *managedDataset {
	return &managedDataset{
		datasetPath: path.Join(storageDir, datasetName),
		token:       token,
		hasher:      sha256.New(),
		client:      new(http.Client),
	}
}

func (d *managedDataset) Sync(ctx context.Context) (bool, error) {
	remoteSum, err := d.getRemoteSum(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get checksum: %w", err)
	}

	localSum, err := d.getLocalSum()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("failed to get local checksum: %w", err)
	}

	if remoteSum == localSum {
		return false, nil
	}

	if err := d.downloadDatabase(ctx); err != nil {
		return false, fmt.Errorf("failed to download database: %w", err)
	}

	return true, nil
}

func (d *managedDataset) newMmdbReader() (*maxminddb.Reader, error) {
	file, err := os.Open(d.datasetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open dataset file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}

	buf := make([]byte, stat.Size())
	n, err := io.ReadFull(file, buf)
	if err != nil {
		if n > 0 {
			return nil, fmt.Errorf("failed to read entire dataset file: %w", err)
		} else if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("dataset file is empty: %w", err)
		} else if n != int(stat.Size()) {
			return nil, fmt.Errorf("read %d bytes but expected %d: %w", n, stat.Size(), err)
		}
		return nil, fmt.Errorf("failed to read dataset file: %w", err)
	}

	reader, err := maxminddb.OpenBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create MMDB reader: %w", err)
	}

	return reader, nil
}

func (d *managedDataset) getLocalSum() (string, error) {
	file, err := os.Open(d.datasetPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(d.hasher, file)
	if err != nil {
		return "", err
	}
	defer d.hasher.Reset()

	return hex.EncodeToString(d.hasher.Sum(nil)), nil
}

func (d *managedDataset) getRemoteSum(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", datasetChecksumURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	} else if resp.Header.Get("Content-Type") != "application/json" {
		return "", fmt.Errorf("unexpected content type: %s", resp.Header.Get("Content-Type"))
	}

	decoder := json.NewDecoder(resp.Body)

	var checksums struct {
		Checksums struct {
			Sha256 string `json:"sha256"`
		} `json:"checksums"`
	}

	if err := decoder.Decode(&checksums); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return checksums.Checksums.Sha256, nil
}

func (d *managedDataset) downloadDatabase(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", datasetDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	} else if resp.ContentLength <= 0 {
		return fmt.Errorf("invalid content length: %d", resp.ContentLength)
	} else if resp.ContentLength > 1<<30 { // 1 GB
		return fmt.Errorf("content length too large: %d", resp.ContentLength)
	} else if resp.Header.Get("Content-Type") != "application/vnd.maxmind.maxmind-db" {
		return fmt.Errorf("unexpected content type: %s", resp.Header.Get("Content-Type"))
	}

	// We need to verify the checksum before writing to disk to avoid leaving a corrupted file
	remoteSum, err := d.getRemoteSum(ctx)
	if err != nil {
		return fmt.Errorf("failed to get remote checksum: %w", err)
	}

	buf := make([]byte, resp.ContentLength)

	_, err = io.ReadFull(resp.Body, buf)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	d.hasher.Write(buf)
	sum := hex.EncodeToString(d.hasher.Sum(nil))
	d.hasher.Reset()

	if sum != remoteSum {
		return &ErrUnmatchedChecksum{
			Expected: remoteSum,
			Actual:   sum,
		}
	}

	out, err := os.Create(d.datasetPath)
	if err != nil {
		return fmt.Errorf("failed to create dataset file: %w", err)
	}
	defer out.Close()

	_, err = out.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write dataset file: %w", err)
	}

	return nil
}
