package icons

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Service struct {
	Dir        string
	DefaultDir string
}

var safePartPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
var dataPNGPattern = regexp.MustCompile(`(?i)^data:image/png;base64,(.+)$`)

// Save 写入 64/256 两个尺寸的自定义图标，返回是否真的写了文件。
// 传进来的 dataURL 解不开时必须报错：之前这里是 continue 跳过、返回 (false, nil)，
// 调用方拿到 nil 就把 DisplayIcon 指向一个并不存在的文件，还顺手递增了版本号。
func (s *Service) Save(appName, icon64, icon256 string) (bool, error) {
	safeAppName := safePart(appName)
	saved := false
	pairs := map[int]string{64: icon64, 256: icon256}
	for size, dataURL := range pairs {
		if dataURL == "" {
			continue
		}
		match := dataPNGPattern.FindStringSubmatch(strings.TrimSpace(dataURL))
		if match == nil {
			return saved, fmt.Errorf("icon %d: 不是 image/png 的 data URL", size)
		}
		data, err := base64.StdEncoding.DecodeString(match[1])
		if err != nil {
			return saved, fmt.Errorf("icon %d: base64 解码失败: %w", size, err)
		}
		if err := s.write(filepath.Join(s.Dir, safeAppName+"-"+itoa(size)+".png"), data); err != nil {
			return saved, err
		}
		saved = true
	}
	return saved, nil
}

// Remove deletes the stored custom icons for an entry. Missing files are
// treated as success so repeated removals stay idempotent.
func (s *Service) Remove(appName string) (bool, error) {
	safeAppName := safePart(appName)
	removed := false
	for _, size := range []int{64, 256} {
		path := filepath.Join(s.Dir, safeAppName+"-"+itoa(size)+".png")
		if err := os.Remove(path); err == nil {
			removed = true
		} else if !os.IsNotExist(err) {
			return removed, err
		}
	}
	return removed, nil
}
func (s *Service) Read(appName string, size int) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, safePart(appName)+"-"+itoa(size)+".png"))
	if err == nil {
		return data, nil
	}
	if size == 64 {
		return s.Fallback(64)
	}
	if size == 256 {
		return s.Fallback(256)
	}
	return nil, err
}

func (s *Service) Fallback(size int) ([]byte, error) {
	if s.DefaultDir != "" {
		name := "child_entry_" + itoa(size) + ".png"
		data, err := os.ReadFile(filepath.Join(s.DefaultDir, name))
		if err == nil {
			return data, nil
		}
	}
	// 回退路径：app/icons/ 和 app/ui/images/
	for _, appRoot := range []string{os.Getenv("SHORTCUTS_APP_ROOT"), os.Getenv("TRIM_APPDEST")} {
		if appRoot == "" {
			continue
		}
		for _, sub := range []string{"app/icons", "icons"} {
			data, err := os.ReadFile(filepath.Join(appRoot, sub, "child_entry_"+itoa(size)+".png"))
			if err == nil {
				return data, nil
			}
		}
	}
	if size == 256 {
		if data, err := os.ReadFile(filepath.Join("shortcuts", "app", "ui", "images", "icon_256.png")); err == nil {
			return data, nil
		}
	}
	if size == 64 {
		if data, err := os.ReadFile(filepath.Join("shortcuts", "app", "ui", "images", "icon_64.png")); err == nil {
			return data, nil
		}
	}
	return solidPNG(size)
}

func (s *Service) write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func safePart(value string) string {
	value = safePartPattern.ReplaceAllString(value, "_")
	if len(value) > 64 {
		value = value[:64]
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := []byte{}
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	if negative {
		return "-" + string(buf)
	}
	return string(buf)
}

func solidPNG(size int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 116, B: 139, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
