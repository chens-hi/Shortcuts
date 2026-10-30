package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/chens-hi/shortcuts/internal/model"
)

// folderInput 构造一个 folder 类型的创建入参。config 与 path 分开传，
// 避免依赖 normalizeInput 的字段合并语义，直接测保存链路的路径校验。
func folderInput(appname, path string) EntryInput {
	raw, err := json.Marshal(model.FolderConfig{Path: path})
	if err != nil {
		panic(err)
	}
	return EntryInput{
		Type:        model.EntryTypeFolder,
		AppName:     appname,
		DisplayName: "照片",
		Config:      raw,
	}
}

// isValidationError 判断错误是否是校验类错误。
func isValidationError(err error) bool {
	var apiErr *model.APIError
	return err != nil && errors.As(err, &apiErr) && apiErr.Code == model.ErrValidation
}

func TestValidateFolderPath(t *testing.T) {
	app := newRecordingAppCenter()
	svc, _ := newDeleteTestService(t, app, nil)

	real := realFolderPath(t, "validate")
	valid, code, err := svc.ValidateFolderPath(real)
	if err != nil || !valid || code != "" {
		t.Fatalf("ValidateFolderPath(real=%q) = (%v, %q, %v), want (true, \"\", nil)", real, valid, code, err)
	}

	// 不存在的路径：格式合法但不存在，必须判无效，返回 not_found 码。
	valid, code, err = svc.ValidateFolderPath(real + "/不存在")
	if err != nil || valid || code != "not_found" {
		t.Fatalf("ValidateFolderPath(fake) = (%v, %q, %v), want (false, \"not_found\", nil)", valid, code, err)
	}

	// 格式非法：相对路径、根目录、空串 → bad_format 码。
	for _, path := range []string{"vol1/1000", "/", ""} {
		valid, code, err := svc.ValidateFolderPath(path)
		if err != nil || valid || code != "bad_format" {
			t.Fatalf("ValidateFolderPath(%q) = (%v, %q, %v), want (false, \"bad_format\", nil)", path, valid, code, err)
		}
	}
}

func TestCreateFolderEntryRejectsNonexistentDir(t *testing.T) {
	app := newRecordingAppCenter()
	svc, _ := newDeleteTestService(t, app, nil)

	real := realFolderPath(t, "create")
	if _, err := svc.CreateEntry(folderInput("photos", real)); err != nil {
		t.Fatalf("CreateEntry(real=%q) = %v, want nil", real, err)
	}

	// 同一条创建链路里，不存在的目录必须被拒绝（保存时校验）。
	if _, err := svc.CreateEntry(folderInput("photos2", real+"/不存在")); err == nil {
		t.Fatal("CreateEntry(nonexistent) = nil, want validation error")
	} else if !isValidationError(err) {
		t.Fatalf("CreateEntry(nonexistent) = %v, want ErrValidation", err)
	}
}

func TestUpdateFolderEntryRejectsNonexistentDir(t *testing.T) {
	app := newRecordingAppCenter()
	svc, _ := newDeleteTestService(t, app, nil)

	real := realFolderPath(t, "update")
	if _, err := svc.CreateEntry(folderInput("photos", real)); err != nil {
		t.Fatalf("seed CreateEntry(real=%q) = %v, want nil", real, err)
	}

	// 更新到不存在的目录：拒绝。
	if _, err := svc.UpdateEntry("photos", folderInput("photos", real+"/不存在")); err == nil {
		t.Fatal("UpdateEntry(nonexistent) = nil, want validation error")
	}
}
