package service

import (
	"context"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Foreign DPI engines detected by process image name; our own winws2.exe is excluded by path.
var foreignImages = map[string]bool{
	"winws.exe":      true,
	"winws2.exe":     true,
	"goodbyedpi.exe": true,
	"byedpi.exe":     true,
}

type processEntry32 struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [windows.MAX_PATH]uint16
}

var (
	procCreateToolhelp32Snapshot = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = windows.NewLazySystemDLL("kernel32.dll").NewProc("Process32FirstW")
	procProcess32NextW           = windows.NewLazySystemDLL("kernel32.dll").NewProc("Process32NextW")
)

func snapshotProcesses() map[uint32]string {
	out := map[uint32]string{}
	h, _, _ := procCreateToolhelp32Snapshot.Call(0x2 /* TH32CS_SNAPPROCESS */, 0)
	if h == 0 || h == uintptr(windows.InvalidHandle) {
		return out
	}
	defer windows.CloseHandle(windows.Handle(h))
	var entry processEntry32
	entry.dwSize = uint32(unsafe.Sizeof(entry))
	ok, _, _ := procProcess32FirstW.Call(h, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		out[entry.th32ProcessID] = windows.UTF16ToString(entry.szExeFile[:])
		ok, _, _ = procProcess32NextW.Call(h, uintptr(unsafe.Pointer(&entry)))
	}
	return out
}

func isForeignImage(name string) bool {
	name = strings.ToLower(name)
	if foreignImages[name] {
		return true
	}
	return strings.HasPrefix(name, "zapret") && strings.HasSuffix(name, ".exe")
}

// sameImagePath compares process image paths case-insensitively, ignoring symlinks.
func sameImagePath(a, b string) bool {
	clean := func(p string) string {
		if real, e := filepath.EvalSymlinks(p); e == nil {
			p = real
		}
		return strings.ToLower(filepath.Clean(p))
	}
	return clean(a) == clean(b)
}

func processImagePath(pid uint32) (string, bool) {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if e != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	var buf [windows.MAX_PATH]uint16
	n := uint32(len(buf))
	if e = windows.QueryFullProcessImageName(h, 0, &buf[0], &n); e != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

func foreignProcesses() map[uint32]string {
	out := map[uint32]string{}
	for pid, name := range snapshotProcesses() {
		if !isForeignImage(name) {
			continue
		}
		if path, ok := processImagePath(pid); ok && sameImagePath(path, engine.Path) {
			continue // наш собственный winws2.exe рядом с fdpi.exe
		}
		out[pid] = name
	}
	return out
}

func sortedNames(procs map[uint32]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range procs {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func OtherActive(ctx context.Context) []string { return sortedNames(foreignProcesses()) }

func terminateProcess(pid uint32) {
	h, e := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if e != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.TerminateProcess(h, 1)
}

func openFdpiService() (*mgr.Mgr, *mgr.Service, error) {
	m, e := mgr.Connect()
	if e != nil {
		return nil, nil, fmt.Errorf("подключение к диспетчеру служб: %w", e)
	}
	s, e := m.OpenService(serviceName)
	if e != nil {
		m.Disconnect()
		return nil, nil, fmt.Errorf("служба fdpi не установлена; выполните fdpi setup: %w", e)
	}
	return m, s, nil
}

func stopFdpiService(s *mgr.Service) error {
	st, e := s.Query()
	if e != nil {
		return e
	}
	if st.State == svc.Stopped {
		return nil
	}
	if _, e = s.Control(svc.Stop); e != nil {
		return fmt.Errorf("остановка службы fdpi: %w", e)
	}
	return waitState(s, svc.Stopped, 15*time.Second)
}

// Control only replaces foreign engines on an explicit --replace request.
func Control(ctx context.Context, action string, replace bool) error {
	if action == "start" || action == "restart" {
		foreign := foreignProcesses()
		if len(foreign) > 0 {
			names := sortedNames(foreign)
			if !replace {
				return fmt.Errorf("%w: %s. Откройте fdpi и подтвердите замену либо выполните fdpi %s --replace; чужие процессы автоматически не остановлены", ErrConflict, strings.Join(names, ", "), action)
			}
			for pid := range foreign {
				terminateProcess(pid)
			}
		}
	}
	m, s, e := openFdpiService()
	if e != nil {
		return e
	}
	defer m.Disconnect()
	defer s.Close()
	switch action {
	case "start":
		return s.Start()
	case "stop":
		return stopFdpiService(s)
	case "restart":
		if e = stopFdpiService(s); e != nil {
			return e
		}
		return s.Start()
	case "enable":
		cfg, e := s.Config()
		if e != nil {
			return e
		}
		cfg.StartType = mgr.StartAutomatic
		return s.UpdateConfig(cfg)
	case "disable":
		cfg, e := s.Config()
		if e != nil {
			return e
		}
		cfg.StartType = mgr.StartManual
		return s.UpdateConfig(cfg)
	default:
		return fmt.Errorf("неизвестное действие службы: %s", action)
	}
}
