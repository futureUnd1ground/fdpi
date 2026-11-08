// Package service supervises nfqws and restores the last accepted configuration.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const SystemConfig = "/etc/fdpi/config.json"

func Command(ctx context.Context, args ...string) (string, error) {
	b, e := exec.CommandContext(ctx, "/usr/bin/systemctl", args...).CombinedOutput()
	if e != nil {
		return string(b), fmt.Errorf("systemctl %v: %w: %s", args, e, b)
	}
	return strings.TrimSpace(string(b)), nil
}
func Lock(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("другая операция FDPI уже использует блокировку %s", path)
	}
	return f, nil
}
func Unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }
func queueReady() bool {
	b, e := os.ReadFile("/proc/net/netfilter/nfnetlink_queue")
	if e != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == strconv.Itoa(network.Queue) {
			return true
		}
	}
	return false
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
	f, e := Lock("/run/fdpi/engine.lock")
	if e != nil {
		return e
	}
	defer Unlock(f)
	c, e := config.Load(path)
	if e != nil {
		return e
	}
	return Supervise(ctx, c, engine.Path, "/run/fdpi")
}
func Supervise(ctx context.Context, c config.Config, enginePath, dir string) (err error) {
	a, tcp, udp, e := engine.ArgsForEngine(c, dir, enginePath)
	if e != nil {
		return e
	}
	if e = engine.Check(ctx, enginePath, a); e != nil {
		return e
	}
	// Stale owned rules from a killed supervisor are removed before binding the queue.
	if e = network.Cleanup(ctx); e != nil {
		return e
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, network.Cleanup(clean))
	}()
	if !tcp && !udp {
		notifyReady()
		<-ctx.Done()
		return nil
	}
	if queueReady() {
		return fmt.Errorf("%w: NFQUEUE %d занята другим процессом", ErrConflict, network.Queue)
	}
	pidfile := filepath.Join(dir, "nfqws.pid")
	if e = os.Remove(pidfile); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	defer os.Remove(pidfile)
	child := exec.Command(enginePath, append(a, "--pidfile="+pidfile)...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if e = child.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			_ = child.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = child.Process.Kill()
				<-done
			}
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
			return fmt.Errorf("nfqws завершился до готовности: %v", e)
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return fmt.Errorf("истекло время ожидания готовности NFQUEUE")
		case <-tick.C:
			b, e := os.ReadFile(pidfile)
			ready = e == nil && strings.TrimSpace(string(b)) == strconv.Itoa(child.Process.Pid)
		}
	}
	if e = network.Apply(ctx, tcp, udp); e != nil {
		return e
	}
	notifyReady()
	fmt.Printf("Обработка FDPI включена: NFQUEUE %d, без слушателя трафик идёт напрямую\n", network.Queue)
	select {
	case <-ctx.Done():
		return nil
	case e = <-done:
		stopped = true
		return fmt.Errorf("nfqws завершился: %v", e)
	}
}
func Daemon() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return Run(ctx, SystemConfig, true)
}
func Apply(ctx context.Context, path string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("для административных действий нужны права root")
	}
	if e := RequireCurrentBinary(ctx); e != nil {
		return e
	}
	if e := engine.Trusted("/etc/fdpi"); e != nil {
		return e
	}
	if e := engine.TrustedLibraries(); e != nil {
		return e
	}
	if e := engine.Trusted(engine.Path); e != nil {
		return e
	}
	if e := engine.Trusted(SystemConfig); e != nil {
		return e
	}
	f, e := Lock("/etc/fdpi/.operation.lock")
	if e != nil {
		return e
	}
	defer Unlock(f)
	c, e := config.Load(path)
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp("/etc/fdpi", ".validate-")
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
		return fmt.Errorf("не удалось проверить службу systemd: %s", state.Detail)
	}
	return commit(SystemConfig, "/etc/fdpi/previous.json", c, state.Service == "active", func() error { return RestartCheck(ctx) }, func() error {
		restore, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return RestartCheck(restore)
	})
}

func commit(path, backup string, c config.Config, active bool, restart, restore func() error) error {
	old, e := config.Load(path)
	if e != nil {
		return e
	}
	if e = config.Save(backup, old); e != nil {
		return e
	}
	if e = config.Save(path, c); e != nil {
		return e
	}
	if active {
		if e = restart(); e != nil {
			return errors.Join(e, config.Save(path, old), restore())
		}
	}
	return nil
}

func RestartCheck(ctx context.Context) error {
	if e := Control(ctx, "restart", false); e != nil {
		return e
	}
	timer := time.NewTimer(800 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	s, e := Command(ctx, "is-active", "fdpi.service")
	if e != nil || s != "active" {
		return fmt.Errorf("служба не запустилась после перезапуска: %s %v", s, e)
	}
	return nil
}

type Status struct {
	Installed       bool   `json:"installed"`
	QueueObservable bool   `json:"queue_observable"`
	Service         string `json:"service"`
	UnitFileState   string `json:"unit_file_state,omitempty"`
	SupervisorPID   int    `json:"supervisor_pid,omitempty"`
	QueueBound      bool   `json:"queue_bound"`
	Detail          string `json:"detail,omitempty"`
}

func State(ctx context.Context) Status {
	s, e := Command(ctx, "show", "fdpi.service", "--property=ActiveState,MainPID,LoadState,UnitFileState")
	_, qe := os.ReadFile("/proc/net/netfilter/nfnetlink_queue")
	v := Status{Service: "unknown", QueueBound: queueReady(), QueueObservable: qe == nil}
	if e != nil {
		v.Detail = e.Error()
		return v
	}
	for _, l := range strings.Split(s, "\n") {
		key, value, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch key {
		case "UnitFileState":
			v.UnitFileState = value
		case "ActiveState":
			v.Service = value
		case "MainPID":
			v.SupervisorPID, _ = strconv.Atoi(value)
		case "LoadState":
			v.Installed = value == "loaded"
		}
	}
	if !v.Installed {
		v.Service = "not-installed"
	}
	return v
}
func JSON(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }

func notifyReady() {
	path := os.Getenv("NOTIFY_SOCKET")
	if path == "" {
		return
	}
	conn, e := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if e != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("READY=1"))
}

func (s Status) QueueText() string {
	if !s.QueueObservable {
		return "неизвестно (нужны права root)"
	}
	if s.QueueBound {
		return "подключена"
	}
	return "не подключена"
}

// StateText renders systemd states for people; JSON keeps the stable systemd identifiers.
func (s Status) StateText() string {
	switch s.Service {
	case "active":
		return "работает"
	case "inactive":
		return "остановлена"
	case "failed":
		return "ошибка запуска"
	case "activating":
		return "запускается"
	case "deactivating":
		return "останавливается"
	case "not-installed":
		return "не установлена — выполните fdpi setup"
	case "unknown":
		return "состояние неизвестно"
	}
	return s.Service
}
