package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// findVMOptionsFiles 查找目录中所有的 .vmoptions 文件
// 优化：简化实现，使用传统循环替代复杂的迭代器链，遵循 KISS 原则
func findVMOptionsFiles(dir string) ([]string, error) {
	// 检查目录读取权限
	if err := checkDirReadPermission(dir); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("无法读取目录 %s: %w", dir, err)
	}

	// 使用简单直观的循环收集匹配的文件路径
	var vmOptionsFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vmoptions") {
			vmOptionsFiles = append(vmOptionsFiles, filepath.Join(dir, entry.Name()))
		}
	}

	return vmOptionsFiles, nil
}

// LineProcessor 定义行处理策略
// 返回 true 表示删除该行，false 表示保留
type LineProcessor func(string) bool

// processVMOptionsFileGeneric 通用的 vmoptions 文件处理函数
// 避免 processVMOptionsFile 和 clearVMOptionsFile 中的代码重复
func processVMOptionsFileGeneric(filePath string, processor LineProcessor, logger *slog.Logger) error {
	// 检查文件权限
	if err := checkFileReadPermission(filePath); err != nil {
		return err
	}

	if err := checkFileWritePermission(filePath); err != nil {
		return err
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 处理行
	lines := strings.Split(string(content), "\n")
	newLines := slices.DeleteFunc(lines, processor)

	// 写回文件
	newContent := strings.Join(newLines, "\n")
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("获取文件权限失败: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(newContent), info.Mode().Perm()); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	logger.Debug("成功更新文件", slog.String("file", filepath.Base(filePath)))
	return nil
}

// processVMOptionsFile 处理单个 vmoptions 文件 - 添加配置
func processVMOptionsFile(filePath, configPath string, logger *slog.Logger) error {
	processor := func(line string) bool {
		trimmed := strings.TrimSpace(line)
		shouldDelete := strings.HasPrefix(trimmed, "--add-opens") ||
			strings.HasPrefix(trimmed, "-javaagent:")
		if shouldDelete {
			logger.Debug("删除行", slog.String("line", trimmed))
		}
		return shouldDelete
	}

	if err := processVMOptionsFileGeneric(filePath, processor, logger); err != nil {
		return err
	}

	// 添加新的配置
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	newConfigs := []string{
		"--add-opens=java.base/jdk.internal.org.objectweb.asm=ALL-UNNAMED",
		"--add-opens=java.base/jdk.internal.org.objectweb.asm.tree=ALL-UNNAMED",
		fmt.Sprintf("-javaagent:%s/ja-netfilter.jar=jetbrains", configPath),
	}

	for _, config := range newConfigs {
		if _, err := file.WriteString(config + "\n"); err != nil {
			return fmt.Errorf("写入配置失败: %w", err)
		}
	}

	logger.Debug("添加配置",
		slog.Int("addOpensCount", 2),
		slog.String("javaagent", configPath+"/ja-netfilter.jar"))

	return nil
}

// clearVMOptionsFile 清除单个 vmoptions 文件中本工具添加的特定配置（不影响用户自定义配置）
func clearVMOptionsFile(filePath string, logger *slog.Logger) error {
	// 临时存储移除的行数
	var removedCount int

	processor := func(line string) bool {
		trimmed := strings.TrimSpace(line)

		// 只删除本工具添加的特定 --add-opens 配置
		if _, exists := toolAddedLines[trimmed]; exists {
			logger.Debug("删除行", slog.String("line", trimmed))
			removedCount++
			return true
		}

		// 只删除包含 ja-netfilter.jar 和 jetbrains 的 javaagent 配置（兼容有引号和无引号格式）
		if strings.HasPrefix(trimmed, "-javaagent:") &&
			strings.Contains(trimmed, "ja-netfilter.jar") &&
			strings.Contains(trimmed, "jetbrains") {
			logger.Debug("删除行", slog.String("line", trimmed))
			removedCount++
			return true
		}

		return false
	}

	if err := processVMOptionsFileGeneric(filePath, processor, logger); err != nil {
		return err
	}

	logger.Debug("成功清除文件",
		slog.String("file", filepath.Base(filePath)),
		slog.Int("removedCount", removedCount))
	return nil
}

// trimTrailingEmptyLines 使用 Go 1.23 slices.Backward 移除尾部空行
func trimTrailingEmptyLines(lines []string) []string {
	trimCount := 0
	for _, line := range slices.Backward(lines) {
		if strings.TrimSpace(line) != "" {
			break
		}
		trimCount++
	}
	if trimCount > 0 {
		return lines[:len(lines)-trimCount]
	}
	return lines
}
