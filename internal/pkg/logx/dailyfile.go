package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wahhheee/aint/internal/pkg/fsx"
)

var timeZero time.Time

// dailyFile 是按天滚动的日志文件写入器：aint-2026-10-20.log。
// 只用标准库实现，够用即可；不做按大小切分。
type dailyFile struct {
	mu     sync.Mutex
	dir    string
	prefix string
	keep   int
	day    string
	f      *os.File
}

func newDailyFile(dir, prefix string, keep int) (*dailyFile, error) {
	if err := os.MkdirAll(dir, fsx.DirPerm); err != nil {
		return nil, err
	}
	d := &dailyFile{dir: dir, prefix: prefix, keep: keep}
	if err := d.rotate(time.Now()); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *dailyFile) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if now := time.Now(); now.Format(time.DateOnly) != d.day {
		if err := d.rotate(now); err != nil {
			return 0, err
		}
	}
	return d.f.Write(p)
}

func (d *dailyFile) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return nil
	}
	err := d.f.Close()
	d.f = nil
	return err
}

// rotate 打开当天的文件，并清理过旧的文件。调用方持有锁。
func (d *dailyFile) rotate(now time.Time) error {
	if d.f != nil {
		_ = d.f.Close()
	}
	d.day = now.Format(time.DateOnly)
	name := filepath.Join(d.dir, fmt.Sprintf("%s-%s.log", d.prefix, d.day))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, fsx.PrivateFilePerm)
	if err != nil {
		return err
	}
	d.f = f
	d.cleanup()
	return nil
}

func (d *dailyFile) cleanup() {
	matches, err := filepath.Glob(filepath.Join(d.dir, d.prefix+"-*.log"))
	if err != nil || len(matches) <= d.keep {
		return
	}
	// 文件名含日期，字典序即时间序。
	slices.SortFunc(matches, strings.Compare)
	for _, m := range matches[:len(matches)-d.keep] {
		_ = os.Remove(m)
	}
}
