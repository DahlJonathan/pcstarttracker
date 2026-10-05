//go:build windows

package winsvc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/sys/windows"
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

const (
	powerSuspend         = 4
	powerResumeSuspend   = 7
	powerResumeAutomatic = 18
)

// Execute is called by the SCM. It accepts STOP, SHUTDOWN and PRESHUTDOWN so the
// agent can emit a graceful shutdown event before Windows kills the process.
func (h *handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPreShutdown | svc.AcceptPowerEvent

	status <- svc.Status{State: svc.StartPending}

	var cancel context.CancelFunc
	var done chan struct{}
	suspended := false
	startRunner := func() {
		ctx, stop := context.WithCancel(context.Background())
		cancel = stop
		done = make(chan struct{})
		go func(completed chan struct{}) {
			h.runner.Run(ctx)
			close(completed)
		}(done)
	}
	stopRunner := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
			done = nil
		}
	}
	startRunner()
	defer stopRunner()

	status <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case <-done:
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.PowerEvent:
				switch c.EventType {
				case powerSuspend:
					if !suspended {
						log.Printf("power suspend: recording inactive state")
						stopRunner()
						h.runner.SendShutdown()
						suspended = true
					}
				case powerResumeSuspend, powerResumeAutomatic:
					if suspended {
						log.Printf("power resume: recording active state")
						suspended = false
						startRunner()
					}
				}
			case svc.Stop, svc.Shutdown, svc.PreShutdown:
				log.Printf("service control %d: stopping", c.Cmd)
				// Tell the SCM we need a moment, then send the final event.
				status <- svc.Status{State: svc.StopPending, WaitHint: 10000}
				stopRunner()
				if !suspended {
					h.runner.SendShutdown()
				}
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

	c := mgr.Config{
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
		ErrorControl:     mgr.ErrorNormal,
		DisplayName:      "PC Status Agent",
		Description:      "Reports this PC's power status to the PC Status app.",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: false,
	}
	s, err := m.OpenService(ServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		s, err = m.CreateService(ServiceName, exePath, c, "run")
	} else if err == nil {
		defer s.Close()
		st, queryErr := s.Query()
		if queryErr != nil {
			return queryErr
		}
		if st.State != svc.Stopped {
			return fmt.Errorf("stop service %s before updating it", ServiceName)
		}
		c.BinaryPathName = windows.EscapeArg(exePath) + " run"
		if err := s.UpdateConfig(c); err != nil {
			return fmt.Errorf("update service: %w", err)
		}
		return configureRecovery(s)
	}
	if err != nil {
		return err
	}
	defer s.Close()
	return configureRecovery(s)
}

func configureRecovery(s *mgr.Service) error {
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 86400); err != nil {
		return fmt.Errorf("configure service recovery: %w", err)
	}
	return s.SetRecoveryActionsOnNonCrashFailures(true)
}

// Uninstall removes the Windows service.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
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
	if err := s.Start("run"); err != nil {
		return err
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == svc.Running {
			return nil
		}
		if st.State == svc.Stopped {
			return fmt.Errorf("service stopped during startup (exit=%d); check %s\\agent.log", st.Win32ExitCode, config.Dir())
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for service to run; check %s\\agent.log", config.Dir())
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// Stop stops the installed service, waiting briefly for it to settle.
func Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer s.Close()

	current, err := s.Query()
	if err != nil {
		return err
	}
	if current.State == svc.Stopped {
		return nil
	}
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
