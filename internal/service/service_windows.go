// Package service supervises winws2 and restores the last accepted configuration.
package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "fdpi"
const serviceDisplay = "FDPI — обход DPI"
const serviceDescription = "Обход DPI через winws2 (WinDivert); управляется FDPI"

// SystemConfig and PreviousConfig live under %ProgramData%\FDPI by default.
var SystemConfig = filepath.Join(StateDir(), "config.json")
var PreviousConfig = filepath.Join(StateDir(), "previous.json")

// StateDir is %ProgramData%\FDPI, or data\ next to fdpi.exe when ProgramData is unset.
func StateDir() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "FDPI")
	}
	exe, e := os.Executable()
	if e != nil {
		return "data"
	}
	return filepath.Join(filepath.Dir(exe), "data")
}
func LogsDir() string { return filepath.Join(StateDir(), "logs") }

// OperationLockPath сериализует административные операции FDPI.
func OperationLockPath() string { return filepath.Join(StateDir(), ".operation.lock") }
func LogPath() string           { return filepath.Join(LogsDir(), "engine.log") }
func EnsureStateDir() error {
	if e := os.MkdirAll(LogsDir(), 0755); e != nil {
		return e
	}
	return nil
}

// IsAdmin reports whether the process token is elevated.
func IsAdmin() bool {
	var token windows.Token
	if e := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); e != nil {
		return false
	}
	defer token.Close()
	var elevation uint32
	var out uint32
	e := windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &out)
	return e == nil && elevation != 0
}

func Lock(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	var overlapped windows.Overlapped
	e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if e != nil {
		f.Close()
		return nil, fmt.Errorf("другая операция FDPI уже использует блокировку %s", path)
	}
	return f, nil
}
func Unlock(f *os.File) {
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
	_ = f.Close()
}

func Run(ctx context.Context, path string, requireTrusted bool) (err error) {
	if foreign := OtherActive(ctx); len(foreign) > 0 {
		return fmt.Errorf("%w: активны %s; подтвердите их остановку в fdpi", ErrConflict, strings.Join(foreign, ", "))
	}
	if requireTrusted {
		if err = engine.Trusted(path); err != nil {
			return
		}
		if err = engine.TrustedLibraries(); err != nil {
			return
		}
		if err = engine.Trusted(engine.Path); err != nil {
			return
		}
	}
	if err = EnsureStateDir(); err != nil {
		return
	}
	f, e := Lock(filepath.Join(StateDir(), "engine.lock"))
	if e != nil {
		return e
	}
	defer Unlock(f)
	c, e := config.Load(path)
	if e != nil {
		return e
	}
	return Supervise(ctx, c, engine.Path, StateDir())
}

func Supervise(ctx context.Context, c config.Config, enginePath, dir string) (err error) {
	a, tcp, udp, e := engine.ArgsForEngine(c, dir, enginePath)
	if e != nil {
		return e
	}
	if e = engine.Check(ctx, enginePath, a); e != nil {
		return e
	}
	if !tcp && !udp {
		notifyReady()
		<-ctx.Done()
		return nil
	}
	if e = EnsureStateDir(); e != nil {
		return e
	}
	pidfile := filepath.Join(dir, "nfqws.pid")
	if e = os.Remove(pidfile); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	defer os.Remove(pidfile)
	logFile, e := os.OpenFile(LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer logFile.Close()
	child := exec.Command(enginePath, append(a, "--pidfile="+pidfile)...)
	out := io.MultiWriter(logFile, os.Stdout)
	child.Stdout = out
	child.Stderr = out
	if e = child.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			_ = child.Process.Kill()
			<-done
		}
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	ready := false
	for !ready {
		select {
		case e = <-done:
			stopped = true
			return fmt.Errorf("winws2 завершился до готовности: %v", e)
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return fmt.Errorf("истекло время ожидания готовности winws2")
		case <-tick.C:
			b, e := os.ReadFile(pidfile)
			ready = e == nil && strings.TrimSpace(string(b)) == strconv.Itoa(child.Process.Pid)
		}
	}
	notifyReady()
	fmt.Printf("Обработка FDPI включена: WinDivert фильтр в winws2, журнал %s\n", LogPath())
	select {
	case <-ctx.Done():
		return nil
	case e = <-done:
		stopped = true
		return fmt.Errorf("winws2 завершился: %v", e)
	}
}

// Daemon runs under the Service Control Manager when invoked by SCM, otherwise directly.
func Daemon() error {
	isService, e := svc.IsWindowsService()
	if e != nil {
		return e
	}
	if !isService {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		return Run(ctx, SystemConfig, true)
	}
	return svc.Run(serviceName, &daemonHandler{})
}

type daemonHandler struct{}

func (h *daemonHandler) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = Run(ctx, SystemConfig, true)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-r:
			if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
				cancel()
				select {
				case <-done:
				case <-time.After(20 * time.Second):
				}
				status <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		case <-done:
			status <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}

// Install registers the fdpi service with automatic start; existing service is reconfigured.
func Install(ctx context.Context) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if exe, e = filepath.Abs(exe); e != nil {
		return e
	}
	m, e := mgr.Connect()
	if e != nil {
		return fmt.Errorf("подключение к диспетчеру служб: %w", e)
	}
	defer m.Disconnect()
	if s, e := m.OpenService(serviceName); e == nil {
		defer s.Close()
		cfg, e := s.Config()
		if e != nil {
			return e
		}
		cfg.DisplayName = serviceDisplay
		cfg.Description = serviceDescription
		cfg.StartType = mgr.StartAutomatic
		return s.UpdateConfig(cfg)
	}
	s, e := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: serviceDisplay,
		Description: serviceDescription,
		StartType:   mgr.StartAutomatic,
	}, "daemon")
	if e != nil {
		return fmt.Errorf("создание службы fdpi: %w", e)
	}
	return s.Close()
}

// Remove stops and deletes the fdpi service.
func Remove(ctx context.Context) error {
	m, e := mgr.Connect()
	if e != nil {
		return fmt.Errorf("подключение к диспетчеру служб: %w", e)
	}
	defer m.Disconnect()
	s, e := m.OpenService(serviceName)
	if e != nil {
		return fmt.Errorf("служба fdpi не установлена: %w", e)
	}
	defer s.Close()
	if st, e := s.Query(); e == nil && st.State != svc.Stopped {
		if _, e = s.Control(svc.Stop); e == nil {
			_ = waitState(s, svc.Stopped, 15*time.Second)
		}
	}
	return s.Delete()
}

func Apply(ctx context.Context, path string) error {
	if !IsAdmin() {
		return fmt.Errorf("для административных действий нужны права администратора")
	}
	if e := RequireCurrentBinary(ctx); e != nil {
		return e
	}
	if e := engine.TrustedLibraries(); e != nil {
		return e
	}
	if e := engine.Trusted(engine.Path); e != nil {
		return e
	}
	if e := EnsureStateDir(); e != nil {
		return e
	}
	f, e := Lock(filepath.Join(StateDir(), ".operation.lock"))
	if e != nil {
		return e
	}
	defer Unlock(f)
	c, e := config.Load(path)
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp(StateDir(), ".validate-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	a, _, _, e := engine.Args(c, dir)
	if e != nil {
		return e
	}
	if e = engine.Check(ctx, engine.Path, a); e != nil {
		return e
	}
	state := State(ctx)
	if state.Detail != "" || !state.Installed {
		return fmt.Errorf("не удалось проверить службу fdpi: %s", state.Detail)
	}
	return commit(SystemConfig, PreviousConfig, c, state.Service == "active", func() error { return RestartCheck(ctx) }, func() error {
		restore, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return RestartCheck(restore)
	})
}

func RestartCheck(ctx context.Context) error {
	if e := Control(ctx, "restart", false); e != nil {
		return e
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		st := State(ctx)
		if st.Detail == "" && st.Service == "active" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("служба не запустилась после перезапуска: %s", st.StateText())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}

func State(ctx context.Context) Status {
	v := Status{Service: "unknown"}
	m, e := mgr.Connect()
	if e != nil {
		v.Detail = e.Error()
		return v
	}
	defer m.Disconnect()
	v.QueueObservable = true
	s, e := m.OpenService(serviceName)
	if e != nil {
		v.Service = "not-installed"
		return v
	}
	defer s.Close()
	v.Installed = true
	st, e := s.Query()
	if e != nil {
		v.Detail = e.Error()
		return v
	}
	switch st.State {
	case svc.Running:
		v.Service = "active"
	case svc.Stopped:
		v.Service = "inactive"
	case svc.StartPending:
		v.Service = "activating"
	case svc.StopPending:
		v.Service = "deactivating"
	}
	pidFile := filepath.Join(StateDir(), "nfqws.pid")
	if b, e := os.ReadFile(pidFile); e == nil {
		v.SupervisorPID, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		v.QueueBound = v.Service == "active"
	}
	return v
}

func waitState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		st, e := s.Query()
		if e != nil {
			return e
		}
		if st.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("истекло время ожидания состояния службы %d", want)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func notifyReady() {}
