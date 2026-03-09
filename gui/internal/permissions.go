package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── 화이트리스트 ──

// whitelistedTools: 항상 자동 허용되는 도구
var whitelistedTools = map[string]bool{
	"Read": true,
	"Glob": true,
	"Grep": true,
}

// whitelistedCommands: Bash 도구에서 자동 허용되는 명령어 접두사
var whitelistedCommands = []string{
	"claudestra",
	"ls", "cat", "head", "tail", "find", "tree", "wc", "file", "stat",
	"grep", "rg", "ag", "fd",
	"echo", "printf", "pwd", "which", "env", "date",
}

// whitelistedGitSubcommands: git의 읽기 전용 서브커맨드
var whitelistedGitSubcommands = []string{
	"status", "log", "diff", "branch", "show", "tag", "remote",
	"stash list", "describe", "rev-parse", "ls-files", "ls-tree",
}

// IsWhitelisted checks if a tool call should be auto-allowed.
func IsWhitelisted(toolName string, toolInput map[string]interface{}) bool {
	// 도구 자체가 화이트리스트에 있으면 허용
	if whitelistedTools[toolName] {
		return true
	}

	// Bash 명령어 체크
	if toolName == "Bash" {
		command, _ := toolInput["command"].(string)
		return isWhitelistedCommand(command)
	}

	return false
}

func isWhitelistedCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	// 첫 번째 토큰 추출 (파이프/세미콜론 이전)
	firstCmd := extractFirstCommand(command)

	// 첫 번째 단어 (실행 파일) 추출 — basename도 체크 (풀 경로 대응)
	firstWord := firstCmd
	if idx := strings.Index(firstCmd, " "); idx >= 0 {
		firstWord = firstCmd[:idx]
	}
	baseName := filepath.Base(firstWord)

	// 화이트리스트 명령어 체크: 명령 이름 또는 basename으로 매칭
	for _, wl := range whitelistedCommands {
		if baseName == wl || firstCmd == wl || strings.HasPrefix(firstCmd, wl+" ") {
			return true
		}
	}

	// git 읽기 전용 서브커맨드 체크 (풀 경로 git도 대응)
	if baseName == "git" || strings.HasPrefix(firstCmd, "git ") {
		// "git " 이후 또는 "/usr/bin/git " 이후 추출
		gitArgs := ""
		if idx := strings.Index(firstCmd, "git "); idx >= 0 {
			gitArgs = firstCmd[idx+4:]
		}
		for _, sub := range whitelistedGitSubcommands {
			if gitArgs == sub || strings.HasPrefix(gitArgs, sub+" ") {
				return true
			}
		}
	}

	return false
}

// extractFirstCommand gets the first command from a pipeline or chain.
func extractFirstCommand(cmd string) string {
	// 파이프, 세미콜론, && 등으로 분리된 첫 번째 명령
	for _, sep := range []string{"|", "&&", "||", ";"} {
		if idx := strings.Index(cmd, sep); idx >= 0 {
			cmd = strings.TrimSpace(cmd[:idx])
			break
		}
	}
	return cmd
}

// ── 권한 요청/응답 파일 ──

type PermissionRequest struct {
	ID        string `json:"id"`
	Tool      string `json:"tool"`
	Command   string `json:"command"`   // Bash 명령어 또는 파일 경로
	Agent     string `json:"agent"`     // 어떤 에이전트가 요청했는지
	Timestamp string `json:"timestamp"`
}

type PermissionResponse struct {
	ID      string `json:"id"`
	Allowed bool   `json:"allowed"`
}

// PermissionsDir returns the permissions directory path for a workspace root.
func PermissionsDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".orchestra", "permissions")
}

// WriteRequest writes a permission request file.
func WriteRequest(dir string, req *PermissionRequest) error {
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "request-"+req.ID+".json"), data, 0644)
}

// ReadResponse reads a permission response file.
func ReadResponse(dir, id string) (*PermissionResponse, error) {
	data, err := os.ReadFile(filepath.Join(dir, "response-"+id+".json"))
	if err != nil {
		return nil, err
	}
	var resp PermissionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WriteResponse writes a permission response file.
func WriteResponse(dir string, resp *PermissionResponse) error {
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "response-"+resp.ID+".json"), data, 0644)
}

// WaitForResponse polls for a response file with timeout.
func WaitForResponse(dir, id string, timeout time.Duration) (*PermissionResponse, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := ReadResponse(dir, id)
		if err == nil {
			return resp, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for permission response: %s", id)
}

// CleanupPermission removes request and response files for an ID.
func CleanupPermission(dir, id string) {
	os.Remove(filepath.Join(dir, "request-"+id+".json"))
	os.Remove(filepath.Join(dir, "response-"+id+".json"))
}
