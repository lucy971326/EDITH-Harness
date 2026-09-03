package web

import (
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestSettingsSectionRegistryRegistersAndSorts(t *testing.T) {
	registry := newSettingsSectionRegistry()
	for _, definition := range []SettingsSectionDefinition{
		{ID: "second", Title: "第二项", Order: 20},
		{ID: "first", Title: "第一项", Order: 10},
		{ID: "alpha", Title: "甲项", Order: 10},
	} {
		if err := registry.RegisterSettingsSection(testSettingsSection{definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	sections := registry.SettingsSections()
	if len(sections) != 3 || sections[0].ID != "alpha" || sections[1].ID != "first" || sections[2].ID != "second" {
		t.Fatalf("sections = %#v", sections)
	}
	if _, ok := registry.SettingsSection("first"); !ok {
		t.Fatal("first section is missing")
	}
}

func TestSettingsSectionRegistryRejectsInvalidAndDuplicate(t *testing.T) {
	registry := newSettingsSectionRegistry()
	if err := registry.RegisterSettingsSection(nil); err == nil {
		t.Fatal("RegisterSettingsSection(nil) succeeded")
	}
	for _, definition := range []SettingsSectionDefinition{
		{ID: "", Title: "empty id"},
		{ID: "Upper", Title: "uppercase"},
		{ID: "7starts", Title: "number"},
		{ID: "has space", Title: "space"},
		{ID: "appearance", Title: "reserved"},
		{ID: "valid", Title: "   "},
	} {
		if err := registry.RegisterSettingsSection(testSettingsSection{definition: definition}); err == nil {
			t.Fatalf("RegisterSettingsSection(%#v) succeeded", definition)
		}
	}
	valid := testSettingsSection{definition: SettingsSectionDefinition{ID: "demo", Title: "演示", Order: 1}}
	if err := registry.RegisterSettingsSection(valid); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterSettingsSection(valid); err == nil {
		t.Fatal("RegisterSettingsSection accepted duplicate")
	}
}

type testSettingsSection struct {
	definition SettingsSectionDefinition
}

func (s testSettingsSection) Definition() SettingsSectionDefinition {
	return s.definition
}

func (testSettingsSection) Render() (templ.Component, error) {
	return templ.Raw("<div>demo content</div>"), nil
}

type errSettingsSection struct {
	definition SettingsSectionDefinition
}

func (s errSettingsSection) Definition() SettingsSectionDefinition {
	return s.definition
}

func (errSettingsSection) Render() (templ.Component, error) {
	return nil, errors.New("render failed")
}

type nilSettingsSection struct {
	definition SettingsSectionDefinition
}

func (s nilSettingsSection) Definition() SettingsSectionDefinition {
	return s.definition
}

func (nilSettingsSection) Render() (templ.Component, error) {
	return nil, nil
}

func TestSettingsHandlerRendersAndIsolatesErrors(t *testing.T) {
	reg := newRegistry()
	if err := registerWebRoutes(reg); err != nil {
		t.Fatal(err)
	}

	// 1. 无任何插件登记时，访问 /settings 显示 Web 自带的外观设置
	req := httptest.NewRequest(nethttp.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	reg.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("appearance status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="theme-select"`) {
		t.Fatalf("expected appearance settings, got: %s", body)
	}

	// 2. 登记一个正常 section 和一个返回错误的 section 以及一个返回 nil 的 section
	if err := reg.RegisterSettingsSection(testSettingsSection{
		definition: SettingsSectionDefinition{ID: "normal", Title: "正常项", Order: 10},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterSettingsSection(errSettingsSection{
		definition: SettingsSectionDefinition{ID: "err-item", Title: "错误项", Order: 20},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterSettingsSection(nilSettingsSection{
		definition: SettingsSectionDefinition{ID: "nil-item", Title: "空项", Order: 30},
	}); err != nil {
		t.Fatal(err)
	}

	// 3. 访问 /settings 仍默认展示 Web 自带外观，插件栏目留在左侧
	req = httptest.NewRequest(nethttp.MethodGet, "/settings", nil)
	rec = httptest.NewRecorder()
	reg.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("first item status = %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `href="/settings/normal"`) {
		t.Fatalf("expected link to normal, got: %s", body)
	}

	// 4. HTMX 局部刷新（HX-Target: settings-content）：验证路由解析 {sectionID} 且 OOB 根节点直接是 nav，无重复 div
	req = httptest.NewRequest(nethttp.MethodGet, "/settings/normal", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "settings-content")
	rec = httptest.NewRecorder()
	reg.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("partial status = %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `<nav id="settings-nav"`) || !strings.Contains(body, `hx-swap-oob="outerHTML"`) {
		t.Fatalf("expected nav with hx-swap-oob, got: %s", body)
	}
	if strings.Contains(body, `<div id="settings-nav"`) {
		t.Fatalf("must not contain redundant wrapper div with duplicate settings-nav ID: %s", body)
	}
	if !strings.Contains(body, "<div>demo content</div>") {
		t.Fatalf("expected demo content in partial, got: %s", body)
	}

	// 5. 访问返回错误的 section 隔离异常，展示友好错误提示
	req = httptest.NewRequest(nethttp.MethodGet, "/settings/err-item", nil)
	rec = httptest.NewRecorder()
	reg.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("err section status = %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "设置项暂时无法加载") {
		t.Fatalf("expected error notice, got: %s", body)
	}

	// 6. 访问返回 nil 的 section 隔离空指针，展示友好错误提示
	req = httptest.NewRequest(nethttp.MethodGet, "/settings/nil-item", nil)
	rec = httptest.NewRecorder()
	reg.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("nil section status = %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "设置项暂时无法加载") {
		t.Fatalf("expected nil notice, got: %s", body)
	}
}
