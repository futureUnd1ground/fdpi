package diagnostics

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// dialControl возвращает Control-функцию Dialer, привязывающую сокеты к интерфейсу.
func dialControl(iface string) (func(string, string, syscall.RawConn) error, error) {
	if _, err := net.InterfaceByName(iface); err != nil {
		return nil, err
	}
	return func(_, _ string, c syscall.RawConn) error {
		var err error
		e := c.Control(func(fd uintptr) { err = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, iface) })
		if e != nil {
			return e
		}
		return err
	}, nil
}
