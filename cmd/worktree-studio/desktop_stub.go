//go:build !desktop

package main

import "errors"

// desktopRun is only available in builds made with -tags desktop (macOS).
func desktopRun(string) error {
	return errors.New("this build has no desktop window (build with -tags desktop,production)")
}
