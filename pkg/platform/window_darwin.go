//go:build darwin

package platform

import "fmt"

// NewWindow on macOS delegates to the Web/Native Studio runtime if CGo Cocoa isn't active.
func NewWindow(title string, width, height int) (Window, error) {
	return nil, fmt.Errorf("macOS window uses Web Studio mode")
}
