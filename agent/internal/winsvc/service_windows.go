//go:build windows

package winsvc

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"pc-tracker-agent/internal/agent"
	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

// ServiceName is the Windows service identifier.
const ServiceName = "PCStatusAgent"

// IsWindowsService reports whether the process was launched by the SCM.
func IsWindowsService() (bool, error) { return svc.IsWindowsService() }

// handler implements svc.Handler and bridges the SCM to the agent Runner.
type handler struct {
	cfg    *config.Config
	runner *agent.Runner
}

// Execute is called by the SCM. It accepts STOP, SHUTDOWN and PRESHUTDOWN so the
// agent can emit a graceful shutdown event before Windows kills the process.
func (h *handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPreShutdown

	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.runner.Run(ctx) // boot event + heartbeats
		close(done)
	}()

	status <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case <-done:
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown, svc.PreShutdown:
				// Tell the SCM we need a moment, then send the final event.
				status <- svc.Status{State: svc.StopPending, WaitHint: 10000}
				cancel()
				h.runner.SendShutdown()
				return false, 0
			default:
				log.Printf("unexpected control request: %d", c.Cmd)
			}
		}
	}
}

// RunService runs the agent under the SCM. Call only when IsWindowsService is true.
func RunService(cfg *config.Config, client *api.Client) error {
	return svc.Run(ServiceName, &handler{cfg: cfg, runner: agent.NewRunner(cfg, client)})
}

// --- install / control helpers ---------------------------------------------

// Install registers the agent as an auto-start Windows service.
func Install(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	if s, err := m.OpenService(ServiceName); err == nil {
		s.Close()
		return fmt.Errorf("service %s already installed", ServiceName)
	}

	s, err := m.CreateService(ServiceName, exePath, mgr.Config{
		DisplayName:      "PC Status Agent",
		Description:      "Reports this PC's power status to the PC Status app.",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
	}, "run")
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

// Uninstall removes the Windows service.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service %s not installed", ServiceName)
	}
	defer s.Close()
	return s.Delete()
}

// Start starts the installed service.
func Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start("run")
}

// Stop stops the installed service, waiting briefly for it to settle.
func Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()

	st, err := s.Control(svc.Stop)
	if err != nil {
		return err
	}
	timeout := time.Now().Add(10 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timed out waiting for service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		st, err = s.Query()
		if err != nil {
			return err
		}
	}
	return nil
}
