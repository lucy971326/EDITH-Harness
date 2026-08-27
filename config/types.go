package config

// Drawer 是插件在抽屉柜上登记的一格普通配置。
type Drawer struct {
	Name     string            // 抽屉名，小写 ASCII
	Defaults map[string]string // 文件里没有的键用这里
	// Validate 检查合并后的值；失败则登记或写入被拒绝。
	Validate func(values map[string]string) error
}

// Need 是插件声明的一把钥匙。
type Need struct {
	Drawer string // 抽屉名
	Key    string // 钥匙名，如 api_key
}
