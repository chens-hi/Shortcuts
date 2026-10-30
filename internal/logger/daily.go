package logger

import (
	"os"
	"path/filepath"
	"time"
)

// dailyWriter 把日志写到 dir/name_YYYYMMDD.log。
//
// 文件名在每次写入时按当天日期重新计算，所以常驻进程跨天会自然换到新文件。
// 如果只在启动时定一次文件名，服务跑起来就会一直写启动那天的文件，按天保留
// 和过期清理都会失效（该文件 mtime 一直很新，永远删不掉）。
//
// 每次写入都重新打开、写完就关，不长期持有文件句柄：这样清理过期日志时
// 不会因为进程还占着旧文件而删不掉，进程退出也不需要额外收尾。
type dailyWriter struct {
	dir  string
	name string
	now  func() time.Time
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	path := filepath.Join(w.dir, w.name+"_"+w.now().Format("20060102")+".log")
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return os.Stderr.Write(p)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 落盘失败时退回 stderr：宁可进系统日志，也不要静默丢日志。
		return os.Stderr.Write(p)
	}
	defer func() { _ = file.Close() }()
	return file.Write(p)
}
