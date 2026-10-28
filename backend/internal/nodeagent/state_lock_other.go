//go:build !linux

package nodeagent

import (
	"errors"
	"os"
)

func LockStateDirectory(string) (*os.File, error) {
	return nil, errors.New("native execution requires Linux ownership fencing")
}
