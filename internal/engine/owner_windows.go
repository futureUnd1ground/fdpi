package engine

import "os"

// rootOwned has no meaning on Windows; existence and symlink checks are done in Trusted.
func rootOwned(os.FileInfo) bool { return true }
