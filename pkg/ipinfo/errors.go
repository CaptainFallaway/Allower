package ipinfo

import "fmt"

type ErrUnmatchedChecksum struct {
	Expected, Actual string
}

func (e ErrUnmatchedChecksum) Error() string {
	return fmt.Sprintf("unmatched checksum: expected %s, got %s", e.Expected, e.Actual)
}
