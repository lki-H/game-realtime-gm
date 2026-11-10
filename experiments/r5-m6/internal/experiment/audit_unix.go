//go:build !windows

package experiment

import (
	"errors"
	"os"
	"syscall"
)

func secureAuditPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return errors.New("audit_path_requires_current_user_ownership")
	}
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}
