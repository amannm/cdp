package internal

import (
	"os"
	"path/filepath"
)

var (
	BaseDir      = filepath.Join(os.Getenv("HOME"), ".cdp")
	ChromiumDir  = filepath.Join(BaseDir, "chromium")
	InstancesDir = filepath.Join(BaseDir, "instances")
	Verbose      bool
)
