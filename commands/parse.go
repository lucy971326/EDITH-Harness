package commands

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	commandName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	commandLine = regexp.MustCompile(`^/([a-z][a-z0-9_-]*)(?:$|[\t\n\r ])`)
)

// LooksLike 判断去掉首尾空白后是否进入命令平面（以 / 开头）。
func LooksLike(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "/")
}

// Parse 拆开一条斜杠命令。格式不对返回 false，不把它当成聊天。
func Parse(line string) (Parsed, bool) {
	line = strings.TrimSpace(line)
	match := commandLine.FindStringSubmatch(line)
	if match == nil {
		return Parsed{}, false
	}
	name := match[1]
	return Parsed{
		Name:     name,
		RawInput: line[len("/"+name):],
	}, true
}

func normalizeCommand(command Command) (Command, error) {
	command.Name = strings.TrimSpace(command.Name)
	if !commandName.MatchString(command.Name) {
		return Command{}, fmt.Errorf("命令名 %q 必须是小写字母开头的 ASCII 名", command.Name)
	}
	command.Description = strings.TrimSpace(command.Description)
	if command.Description == "" {
		return Command{}, fmt.Errorf("命令 %s 必须有描述", command.Name)
	}
	command.Group = strings.TrimSpace(command.Group)
	command.Hint = strings.TrimSpace(command.Hint)
	if command.Run == nil {
		return Command{}, fmt.Errorf("命令 %s 必须有执行本体", command.Name)
	}
	return command, nil
}
