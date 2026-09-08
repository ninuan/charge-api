package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"charge-dashboard/internal/security"
)

func validateCaptureDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect capture directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("capture directory must be a real directory without group/other write permissions")
	}
	return nil
}

func readCaptureFile(dir, name string) ([]byte, error) {
	filePath := filepath.Join(dir, name)
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, fmt.Errorf("inspect capture file %s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("capture file %s must be regular and not writable by group/other", name)
	}
	if info.Size() > 64<<10 {
		return nil, fmt.Errorf("capture file %s exceeds 64 KiB", name)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("capture file changed while opening")
	}
	return security.ReadLimited(file, 64<<10)
}
