package ipinfo

import (
	"errors"
	"fmt"
)

type ErrUnmatchedChecksum struct {
	Expected, Actual string
}

func (e ErrUnmatchedChecksum) Error() string {
	return fmt.Sprintf("unmatched checksum: expected %s, got %s", e.Expected, e.Actual)
}

var (
	ErrNotFound      = errors.New("address not found in dataset")
	ErrAddrIsPrivate = errors.New("address is private")
)
