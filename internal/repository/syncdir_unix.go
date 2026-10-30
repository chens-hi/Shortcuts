//go:build !windows

package repository

import (
	"os"

	"github.com/chens-hi/shortcuts/internal/model"
)

// syncDir 把目录本身 fsync 一次：rename 之后目录项才算真正落盘。
// 失败要被当作持久化失败上报，不能吞掉。
func syncDir(dirPath string) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return model.Errorf(model.ErrSave, "open data dir failed: dir=%s err=%v", dirPath, err)
	}
	if err := dir.Sync(); err != nil {
		dir.Close()
		return model.Errorf(model.ErrSave, "sync data dir failed: dir=%s err=%v", dirPath, err)
	}
	if err := dir.Close(); err != nil {
		return model.Errorf(model.ErrSave, "close data dir failed: dir=%s err=%v", dirPath, err)
	}
	return nil
}
