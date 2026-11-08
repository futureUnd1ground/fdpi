// Package network on Windows is inert: the WinDivert filter lives and dies inside winws2.exe.
package network

import "context"

const Table = "fdpi"
const Queue = 24000
const Mark = "0x20000000"
const Owner = "fdpi-managed-v1"

func Owned(context.Context) (bool, error)     { return false, nil }
func Apply(context.Context, bool, bool) error { return nil }
func Cleanup(context.Context) error           { return nil }
