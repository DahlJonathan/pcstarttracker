//go:build !windows

package restriction

import "fmt"

type unsupportedScreen struct{}

func newScreen() screen { return &unsupportedScreen{} }
func (*unsupportedScreen) Ensure(_, _ string, locked bool) error {
	if locked {
		return fmt.Errorf("parental restriction requires Windows")
	}
	return nil
}
func (*unsupportedScreen) Close() error { return nil }
