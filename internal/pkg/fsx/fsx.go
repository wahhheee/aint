// Package fsx 提供文件系统辅助函数。
package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFileAtomic 原子地写入文件：先写同目录临时文件并 fsync，再 rename 覆盖目标。
// 崩溃时目标文件要么是旧内容，要么是新内容，不会出现写了一半的文件。
// 用于 blob、secrets.json、instance.json 等。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// syncDir 在类 Unix 系统上 fsync 目录，保证 rename 本身落盘。Windows 不支持也不需要。
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// CheckWritable 通过实际创建并删除一个探测文件来判断目录是否可写。
// 只看权限位不可靠（Windows ACL、只读介质、Program Files 虚拟化等）。
func CheckWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return fmt.Errorf("目录 %s 不可写: %w", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}
