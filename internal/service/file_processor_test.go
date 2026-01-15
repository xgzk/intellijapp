package service

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindVMOptionsFiles 测试查找 .vmoptions 文件
func TestFindVMOptionsFiles(t *testing.T) {
	// 创建临时测试目录
	tempDir := t.TempDir()

	// 创建测试文件
	// 注意：Windows 文件系统大小写不敏感，idea.vmoptions 和 IDEA.vmoptions 会被认为是同一个文件
	testFiles := []string{
		"idea64.vmoptions",
		"idea.vmoptions",
		"other.txt",         // 非 vmoptions 文件
		"test.VMOPTIONS",    // 测试大小写扩展名
	}

	expectedCount := 3 // 应该找到 3 个 .vmoptions 文件

	for _, file := range testFiles {
		path := filepath.Join(tempDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("无法创建测试文件 %s: %v", file, err)
		}
	}

	// 创建子目录（不应该被包含）
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("无法创建子目录: %v", err)
	}

	// 执行测试
	files, err := findVMOptionsFiles(tempDir)
	if err != nil {
		t.Fatalf("findVMOptionsFiles 返回错误: %v", err)
	}

	if len(files) != expectedCount {
		t.Errorf("期望找到 %d 个文件，实际找到 %d 个", expectedCount, len(files))
	}

	// 验证所有返回的文件都是 .vmoptions 文件
	for _, file := range files {
		if !strings.HasSuffix(strings.ToLower(filepath.Base(file)), ".vmoptions") {
			t.Errorf("返回的文件不是 .vmoptions: %s", file)
		}
	}
}

// TestTrimTrailingEmptyLines 测试移除尾部空行
func TestTrimTrailingEmptyLines(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "没有尾部空行",
			input:    []string{"line1", "line2", "line3"},
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "有尾部空行",
			input:    []string{"line1", "line2", "", ""},
			expected: []string{"line1", "line2"},
		},
		{
			name:     "只有空行",
			input:    []string{"", "", ""},
			expected: []string{},
		},
		{
			name:     "空切片",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "尾部有空格的行",
			input:    []string{"line1", "   ", "  "},
			expected: []string{"line1"},
		},
		{
			name:     "中间有空行",
			input:    []string{"line1", "", "line2", ""},
			expected: []string{"line1", "", "line2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimTrailingEmptyLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("长度不匹配: got %d, expected %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("索引 %d 不匹配: got %q, expected %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestProcessVMOptionsFile 测试处理 vmoptions 文件
func TestProcessVMOptionsFile(t *testing.T) {
	// 创建临时测试目录
	tempDir := t.TempDir()
	rawConfigPath := filepath.Join(tempDir, "config")

	// 创建配置目录
	if err := os.Mkdir(rawConfigPath, 0755); err != nil {
		t.Fatalf("无法创建配置目录: %v", err)
	}

	normalizedConfigPath := filepath.ToSlash(rawConfigPath)

	// 创建测试文件
	vmFile := filepath.Join(tempDir, "test.vmoptions")
	initialContent := `-Xmx2048m
-Xms512m
--add-opens=java.base/java.lang=ALL-UNNAMED
-javaagent:/old/path/ja-netfilter.jar=jetbrains`

	if err := os.WriteFile(vmFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("无法创建测试文件: %v", err)
	}

	// 执行处理
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := processVMOptionsFile(vmFile, normalizedConfigPath, logger); err != nil {
		t.Fatalf("处理文件失败: %v", err)
	}

	// 读取处理后的内容
	content, err := os.ReadFile(vmFile)
	if err != nil {
		t.Fatalf("无法读取处理后的文件: %v", err)
	}

	contentStr := string(content)

	// 验证原始的 -Xmx 和 -Xms 参数仍然存在
	if !strings.Contains(contentStr, "-Xmx2048m") {
		t.Error("原始的 -Xmx 参数丢失")
	}
	if !strings.Contains(contentStr, "-Xms512m") {
		t.Error("原始的 -Xms 参数丢失")
	}

	// 验证旧的 javaagent 被移除
	if strings.Contains(contentStr, "-javaagent:/old/path/ja-netfilter.jar=jetbrains") {
		t.Error("旧的 javaagent 配置未被移除")
	}

	// 验证新的 add-opens 被添加（且仅一次）
	if strings.Count(contentStr, "--add-opens=java.base/jdk.internal.org.objectweb.asm=ALL-UNNAMED") != 1 {
		t.Error("add-opens asm 结果不符合预期")
	}
	if strings.Count(contentStr, "--add-opens=java.base/jdk.internal.org.objectweb.asm.tree=ALL-UNNAMED") != 1 {
		t.Error("add-opens asm.tree 结果不符合预期")
	}

	// 验证新的 javaagent 被添加
	expectedAgent := "-javaagent:" + normalizedConfigPath + "/ja-netfilter.jar=jetbrains"
	if !strings.Contains(contentStr, expectedAgent) {
		t.Errorf("新的 javaagent 配置缺失: %s", expectedAgent)
	}
}
