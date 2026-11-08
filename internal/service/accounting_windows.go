package service

import "context"

// EnableAccounting is a no-op on Windows: winws2 keeps its own connection tracking.
func EnableAccounting(ctx context.Context) error { return nil }

// PersistAccounting is a no-op on Windows: no sysctl persistence exists.
func PersistAccounting() error { return nil }
