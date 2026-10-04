//go:build !windows

package winsvc

import (
	"errors"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

// ServiceName is the service identifier (unused off Windows).
const ServiceName = "PCStatusAgent"

var errUnsupported = errors.New("Windows service control is only available on Windows")

// IsWindowsService always reports false on non-Windows platforms.
func IsWindowsService() (bool, error) { return false, nil }

func RunService(_ *config.Config, _ *api.Client) error { return errUnsupported }
func Install(_ string) error                           { return errUnsupported }
func Uninstall() error                                 { return errUnsupported }
func Start() error                                     { return errUnsupported }
func Stop() error                                      { return errUnsupported }
