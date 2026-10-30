package provider

import (
	"encoding/json"

	"github.com/chens-hi/shortcuts/internal/model"
)

type ApplicationSpec struct {
	AppName     string
	DisplayName string
	Description string
	Version     string
	OpenMode    string
	UIConfig    any
	CGIScript   string
	// ExtraManifest 追加/覆盖子应用 manifest 的 key=value 行。
	// 与 manifest.tmpl 的固定字段同名时以这里的值为准（如 micro_app、ctl_stop）。
	ExtraManifest map[string]string
	// ExtraFiles 写入子包 app/ 目录下的额外文件，键是相对路径，
	// 值是文件内容。用于注入按实例变化又不想走模板渲染的静态文件。
	ExtraFiles map[string]string
	// TemplateVars 在拷贝模板时做字面量替换，键是模板里的占位符，
	// 值是替换后的内容。用于把按实例变化的数据烘焙进静态页，
	// 避免页面再发一次请求去取（文件夹跳转页的目标路径）。
	TemplateVars map[string]string
}

type EntryProvider interface {
	Type() model.EntryType
	Validate(entry model.Entry) error
	Spec(entry model.Entry, mainAppname string) (ApplicationSpec, error)
	Changed(old model.Entry, next model.Entry) bool
}

type Registry struct {
	providers map[model.EntryType]EntryProvider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[model.EntryType]EntryProvider)}
}

func (r *Registry) Register(entryProvider EntryProvider) {
	r.providers[entryProvider.Type()] = entryProvider
}

func (r *Registry) Get(entryType model.EntryType) (EntryProvider, error) {
	entryProvider, ok := r.providers[entryType]
	if !ok {
		return nil, model.Errorf(model.ErrValidation, "unknown entry type: %s", entryType)
	}
	return entryProvider, nil
}

func DecodeConfig[T any](raw json.RawMessage) (T, error) {
	var out T
	if len(raw) == 0 {
		return out, model.Errorf(model.ErrValidation, "config is required")
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, model.Errorf(model.ErrValidation, "invalid config: %v", err)
	}
	return out, nil
}
