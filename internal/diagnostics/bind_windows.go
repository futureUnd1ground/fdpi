package diagnostics

import (
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

// ipUnicastIf — IP_UNICAST_IF из ws2ipdef.h (в x/sys/windows нет готовой константы).
const ipUnicastIf = 31

// dialControl возвращает Control-функцию Dialer, привязывающую исходящие
// IPv4-сокеты к интерфейсу (IP_UNICAST_IF). Для IPv6-сокетов эта опция
// может не применяться — ошибки игнорируются, чтобы не ломать дуалы.
func dialControl(iface string) (func(string, string, syscall.RawConn) error, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	index := ifi.Index
	return func(_, _ string, c syscall.RawConn) error {
		var err error
		e := c.Control(func(fd uintptr) {
			err = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, ipUnicastIf, index)
		})
		if e != nil {
			return e
		}
		return err
	}, nil
}
