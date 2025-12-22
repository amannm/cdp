package internal

import (
	"os"
	"path/filepath"
)

var (
	BaseDir      = filepath.Join(os.Getenv("HOME"), ".cdp")
	ChromeDir    = filepath.Join(BaseDir, "chrome")
	InstancesDir = filepath.Join(BaseDir, "instances")
	Verbose      bool
)
