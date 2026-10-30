package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/chens-hi/shortcuts/internal/model"
)

// entryKey 长度有限（folder:<序号>），请求体上限给足余量即可；
// 这里单独限流，避免复用 8MB 上限让一个 3 字节的接口能读 8MB。
const maxDesktopItemBody = 512

func (s *Server) handleDesktopItem(w http.ResponseWriter, r *http.Request, appName string) {
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, maxDesktopItemBody+1))
		if err != nil {
			writeValidationDetail(w, "读取请求体失败: "+err.Error())
			return
		}
		if len(body) > maxDesktopItemBody {
			writeValidationDetail(w, "桌面项标识请求体超长")
			return
		}
		var payload struct {
			EntryKey string `json:"entryKey"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeValidationDetail(w, "请求体不是 JSON 对象: "+err.Error())
			return
		}
		// 清空走 DELETE；PUT 只接受真实 key，"传空即清空"会让误请求静默抹掉凭据。
		if strings.TrimSpace(payload.EntryKey) == "" {
			writeValidationDetail(w, "entryKey is required")
			return
		}
		s.writeDesktopEntry(w, appName, payload.EntryKey)
	case http.MethodDelete:
		s.writeDesktopEntry(w, appName, "")
	default:
		writeError(w, http.StatusMethodNotAllowed, model.ErrMethod)
	}
}

func (s *Server) writeDesktopEntry(w http.ResponseWriter, appName, key string) {
	entry, err := s.Svc.SetDesktopEntryKey(appName, key)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry})
}
