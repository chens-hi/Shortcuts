package repository

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/chens-hi/shortcuts/internal/model"
)

type EntryRepository interface {
	Load() ([]model.Entry, error)
	Save([]model.Entry) error
}

type JSONRepository struct {
	Path string
}

type fileData struct {
	Version int           `json:"version"`
	Entries []model.Entry `json:"entries"`
}

func NewJSONRepository(path string) *JSONRepository {
	return &JSONRepository{Path: path}
}

func (r *JSONRepository) Load() ([]model.Entry, error) {
	raw, err := os.ReadFile(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Entry{}, nil
		}
		return nil, model.Errorf(model.ErrSave, "read data file failed: path=%s parent=%s err=%v", r.Path, filepath.Dir(r.Path), err)
	}
	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		// 结构不符就是错。以前还留了一条「旧版裸数组按兼容格式加载」的分支，
		// 已经明确不需要兼容旧数据，留着只会让脏数据绕过字段校验。
		return nil, model.Errorf(model.ErrSave, "shortcuts.json schema mismatch: path=%s bytes=%d err=%v", r.Path, len(raw), err)
	}
	if data.Entries == nil {
		return []model.Entry{}, nil
	}
	normalized := make([]model.Entry, 0, len(data.Entries))
	for _, entry := range data.Entries {
		entry = entry.Normalize()
		// appname 会被拼进构建与安装路径（filepath.Join(BuildDir, child)），
		// 数据文件里带路径分隔符的脏名字能把 RemoveAll/构建写到 build 之外。
		if !model.AppNameValid(entry.AppName) {
			return nil, model.Errorf(model.ErrSave, "invalid appname in data file: appname=%q path=%s", entry.AppName, r.Path)
		}
		normalized = append(normalized, entry)
	}
	return normalized, nil
}

func (r *JSONRepository) Save(entries []model.Entry) error {
	if entries == nil {
		entries = []model.Entry{}
	}
	data, err := json.MarshalIndent(fileData{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return model.Errorf(model.ErrSave, "marshal data file failed: path=%s err=%v", r.Path, err)
	}
	dirPath := filepath.Dir(r.Path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return model.Errorf(model.ErrSave, "create data dir failed: dir=%s err=%v", dirPath, err)
	}
	tmp, err := os.CreateTemp(dirPath, ".shortcuts-*.tmp")
	if err != nil {
		return model.Errorf(model.ErrSave, "create temp data file failed: dir=%s err=%v", dirPath, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	content := append(data, '\n')
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return model.Errorf(model.ErrSave, "write temp data file failed: path=%s err=%v", tmpName, err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return model.Errorf(model.ErrSave, "chmod temp data file failed: path=%s err=%v", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return model.Errorf(model.ErrSave, "sync temp data file failed: path=%s err=%v", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return model.Errorf(model.ErrSave, "close temp data file failed: path=%s err=%v", tmpName, err)
	}
	if err := os.Rename(tmpName, r.Path); err != nil {
		return model.Errorf(model.ErrSave, "replace data file failed: path=%s tmp=%s err=%v", r.Path, tmpName, err)
	}
	// 目录 fsync 失败同样是持久化失败：以前这些错误全 return nil，
	// rename 之后掉电就没人知道数据到底落没落盘。
	return syncDir(dirPath)
}
