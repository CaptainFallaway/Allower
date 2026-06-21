package config

import "fmt"

type ErrCreatedConfigFile struct {
	Path string
}

func (e ErrCreatedConfigFile) Error() string {
	return fmt.Sprintf("%q has been created, please make changes and restart...", e.Path)
}
