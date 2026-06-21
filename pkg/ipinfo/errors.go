package ipinfo

import (
	"fmt"
	"net/netip"
)

type ErrUnmatchedChecksum struct {
	Expected, Actual string
}

func (e ErrUnmatchedChecksum) Error() string {
	return fmt.Sprintf("unmatched checksum: expected %s, got %s", e.Expected, e.Actual)
}

type ErrIpNotFound struct {
	Addr netip.Addr
}

func (e ErrIpNotFound) Error() string {
	return fmt.Sprintf("address not found: %s", e.Addr)
}

type ErrAddrIsPrivate struct {
	Addr netip.Addr
}

func (e ErrAddrIsPrivate) Error() string {
	return fmt.Sprintf("not a public address: %s", e.Addr)
}
