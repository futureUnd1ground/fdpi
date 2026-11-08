package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"golang.org/x/sys/windows"
)

// isRoot сообщает, обладает ли процесс повышенными правами (UAC).
func isRoot() bool { return service.IsAdmin() }

// tuiForbiddenForRoot: на Windows интерфейс работает и от администратора.
func tuiForbiddenForRoot() bool { return false }

// shellExecuteInfo — SHELLEXECUTEINFOW для ShellExecuteExW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hMonitor     windows.Handle
	hProcess     windows.Handle
}

// elevated выполняет команду с повышением прав через UAC (ShellExecuteEx "runas").
func elevated(ctx context.Context, args ...string) error {
	if isRoot() {
		return run(ctx, args)
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.Abs(exe)
	if e != nil {
		return e
	}
	verb, e := windows.UTF16PtrFromString("runas")
	if e != nil {
		return e
	}
	file, e := windows.UTF16PtrFromString(exe)
	if e != nil {
		return e
	}
	params := ""
	if len(args) > 0 {
		quoted := make([]string, len(args))
		for i, a := range args {
			quoted[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		params = strings.Join(quoted, " ")
	}
	paramsPtr, e := windows.UTF16PtrFromString(params)
	if e != nil {
		return e
	}
	dirPtr, e := windows.UTF16PtrFromString(filepath.Dir(exe))
	if e != nil {
		return e
	}
	const (
		seeMaskNoCloseProcess = 0x40  // SEE_MASK_NOCLOSEPROCESS — получить hProcess для ожидания
		seeMaskFlagNoUI       = 0x400 // SEE_MASK_FLAG_NO_UI — ошибки без диалогов
		swHide                = 0
	)
	sei := &shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: paramsPtr,
		lpDirectory:  dirPtr,
		nShow:        swHide,
	}
	sei.cbSize = uint32(unsafe.Sizeof(*sei))
	shellExecuteExW := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	r, _, callErr := shellExecuteExW.Call(uintptr(unsafe.Pointer(sei)))
	if r == 0 {
		if callErr == windows.ERROR_CANCELLED {
			return fmt.Errorf("действие отменено: повышение прав не подтверждено")
		}
		if callErr != windows.ERROR_SUCCESS {
			return fmt.Errorf("ShellExecuteEx: %v", callErr)
		}
		return fmt.Errorf("ShellExecuteEx не удался (код %d)", sei.hInstApp)
	}
	if sei.hProcess == 0 {
		return nil
	}
	defer windows.CloseHandle(sei.hProcess)
	windows.WaitForSingleObject(sei.hProcess, windows.INFINITE)
	var code uint32
	if e := windows.GetExitCodeProcess(sei.hProcess, &code); e == nil && code != 0 {
		return fmt.Errorf("команда завершилась с кодом %d", code)
	}
	return nil
}

// updateHint — инструкция по обновлению на Windows.
func updateHint() string {
	return "Скачайте новую версию FDPI для Windows и распакуйте поверх каталога программы,\nсохранив рядом с fdpi.exe файлы winws2.exe, WinDivert.dll, WinDivert64.sys и каталог lua.\nЗатем перезапустите службу: fdpi restart.\nЗагрузка и запуск удалённого кода автоматически не выполняются."
}

// adminWindowsSetup устанавливает службу и применяет текущий черновик настроек.
// Вызывается только из процесса с правами администратора (через elevated).
func adminWindowsSetup(ctx context.Context) error {
	if e := service.EnsureStateDir(); e != nil {
		return e
	}
	if e := service.Install(ctx); e != nil {
		return e
	}
	if e := service.Apply(ctx, config.UserPath()); e != nil {
		return e
	}
	return service.Control(ctx, "start", false)
}

// adminWindowsRemove останавливает и удаляет службу FDPI.
func adminWindowsRemove(ctx context.Context) error {
	if !service.IsAdmin() {
		return fmt.Errorf("для удаления службы нужны права администратора")
	}
	_ = service.Control(ctx, "stop", false)
	return service.Remove(ctx)
}
