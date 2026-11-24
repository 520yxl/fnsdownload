//go:build !linux && !windows

package app

import "errors"

func readVolumeSpace(string) (volumeSpace, error) {
	return volumeSpace{}, errors.New("unsupported filesystem platform")
}
