package config

import (
	"fmt"
	"regexp"
	"strings"
)

var slotName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func normalizeDrawer(drawer Drawer) (Drawer, error) {
	drawer.Name = strings.TrimSpace(drawer.Name)
	if !slotName.MatchString(drawer.Name) {
		return Drawer{}, fmt.Errorf("抽屉名 %q 必须是小写字母开头的 ASCII 名", drawer.Name)
	}
	drawer.Defaults = cloneMap(drawer.Defaults)
	for key := range drawer.Defaults {
		if !slotName.MatchString(key) {
			return Drawer{}, fmt.Errorf("抽屉 %s 的键 %q 必须是小写字母开头的 ASCII 名", drawer.Name, key)
		}
	}
	return drawer, nil
}

func normalizeNeed(need Need) (Need, error) {
	need.Drawer = strings.TrimSpace(need.Drawer)
	need.Key = strings.TrimSpace(need.Key)
	if !slotName.MatchString(need.Drawer) {
		return Need{}, fmt.Errorf("抽屉名 %q 必须是小写字母开头的 ASCII 名", need.Drawer)
	}
	if !slotName.MatchString(need.Key) {
		return Need{}, fmt.Errorf("钥匙名 %q 必须是小写字母开头的 ASCII 名", need.Key)
	}
	return need, nil
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneTable(in map[string]map[string]string) map[string]map[string]string {
	if in == nil {
		return map[string]map[string]string{}
	}
	out := make(map[string]map[string]string, len(in))
	for name, row := range in {
		out[name] = cloneMap(row)
	}
	return out
}
