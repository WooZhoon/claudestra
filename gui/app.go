package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gui/internal"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LogEvent is a structured log entry sent to the frontend.
type LogEvent struct {
	Type    string `json:"type"`    // "text", "thinking", "status"
	Message string `json:"message"`
}

type App struct {
	ctx       context.Context
	workspace *internal.Workspace
	lead      *internal.LeadAgent
	agents    map[string]*internal.Agent
	rolePlans []internal.RolePlan
}

func NewApp() *App {
	return &App{
		agents: make(map[string]*internal.Agent),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ── 프로젝트 관리 ──

func (a *App) InitProject(projectDir string) error {
	a.workspace = internal.NewWorkspace(projectDir)
	a.lead = internal.NewLeadAgent(projectDir)
	a.agents = make(map[string]*internal.Agent)
	a.rolePlans = nil
	return nil
}

func (a *App) OpenProject(projectDir string) error {
	ws := internal.NewWorkspace(projectDir)

	config, err := ws.LoadConfig()
	if err != nil {
		return fmt.Errorf("프로젝트를 찾을 수 없습니다. '.orchestra' 폴더가 없습니다. 새 프로젝트로 시작하세요.")
	}

	a.workspace = ws
	a.lead = internal.NewLeadAgent(projectDir)
	a.agents = make(map[string]*internal.Agent)

	// 저장된 RolePlan 복원
	a.rolePlans = ws.LoadRolePlans()
	if len(a.rolePlans) > 0 {
		a.buildTeamFromPlans(a.rolePlans)
	} else if len(config.Agents) > 0 {
		// 레거시 호환: 이전 config.yaml에 roles만 있는 경우
		a.buildLegacyTeam(config.Agents)
	}
	return nil
}

func (a *App) buildTeamFromPlans(plans []internal.RolePlan) {
	lockRegistry := internal.NewFileLockRegistry(a.workspace.LocksDir)
	a.lead = internal.NewLeadAgent(a.workspace.Root)
	a.agents = make(map[string]*internal.Agent)

	// producer 디렉토리 목록 (consumer의 readRefs용)
	var producerDirs []string
	for _, p := range plans {
		if p.Type == "producer" {
			producerDirs = append(producerDirs, filepath.Join(a.workspace.Root, p.Directory))
		}
	}

	for _, plan := range plans {
		agentDir := filepath.Join(a.workspace.Root, plan.Directory)
		isConsumer := plan.Type == "consumer"

		var readRefs []string
		var allowedTools []string
		if isConsumer {
			readRefs = producerDirs
			allowedTools = internal.ConsumerTools
			// doc_writer 등 쓰기가 필요한 consumer
			if strings.Contains(plan.Description, "문서") || strings.Contains(plan.Description, "작성") {
				allowedTools = append(internal.ConsumerTools, "Write")
			}
		} else {
			allowedTools = internal.ProducerTools
		}

		config := internal.AgentConfig{
			AgentID:      plan.Role,
			Role:         plan.Role,
			Idea:         plan.Description,
			WorkDir:      agentDir,
			ReadRefs:     readRefs,
			AllowedTools: allowedTools,
			IsConsumer:   isConsumer,
		}
		agent := internal.NewAgent(config, lockRegistry)
		a.agents[plan.Role] = agent
		a.lead.AddAgent(agent)
	}
}

// 레거시 호환용 (이전 하드코딩 방식 config)
func (a *App) buildLegacyTeam(roles []string) {
	var plans []internal.RolePlan
	for _, role := range roles {
		idea := a.workspace.LoadIdea(role)
		roleType := "producer"
		if role == "reviewer" || role == "doc_writer" {
			roleType = "consumer"
		}
		plans = append(plans, internal.RolePlan{
			Role:        role,
			Description: idea,
			Type:        roleType,
			Directory:   role,
		})
	}
	a.rolePlans = plans
	a.buildTeamFromPlans(plans)
}

// ── 이데아 관리 ──

func (a *App) GetIdea(role string) string {
	if a.workspace == nil {
		return ""
	}
	return a.workspace.LoadIdea(role)
}

func (a *App) UpdateIdea(role, idea string) error {
	if a.workspace == nil {
		return fmt.Errorf("프로젝트가 열려있지 않습니다")
	}
	return a.workspace.SaveIdea(role, idea)
}

// ── 계약서 ──

func (a *App) GetContract() string {
	if a.workspace == nil {
		return ""
	}
	return a.workspace.LoadContract()
}

// ── 상태 조회 ──

type AgentStatusInfo struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	IsConsumer bool   `json:"isConsumer"`
}

type AgentDetailInfo struct {
	ID           string   `json:"id"`
	Role         string   `json:"role"`
	Status       string   `json:"status"`
	IsConsumer   bool     `json:"isConsumer"`
	Instruction  string   `json:"instruction"`
	Output       string   `json:"output"`
	Logs         string   `json:"logs"` // JSONL에서 읽은 실행 로그
	AllowedTools []string `json:"allowedTools"`
}

func (a *App) GetAgentDetail(agentID string) *AgentDetailInfo {
	agent, ok := a.agents[agentID]
	if !ok {
		return nil
	}

	// JSONL 로그에서 instruction, output, 실시간 로그 읽기
	instruction := agent.LastInstruction
	output := agent.Output
	var logs string

	if a.workspace != nil {
		logPath := filepath.Join(a.workspace.LogsDir, agentID+".jsonl")
		if entries, err := readJSONLEntries(logPath); err == nil && len(entries) > 0 {
			// 연속된 같은 타입 청크를 합쳐서 로그 구성
			var logLines []string
			var lastType string
			for _, e := range entries {
				// 연속된 같은 타입이면 마지막 줄에 이어붙임
				if e.Type == lastType && (e.Type == "thinking" || e.Type == "text") && len(logLines) > 0 {
					logLines[len(logLines)-1] += e.Message
					continue
				}
				lastType = e.Type
				switch e.Type {
				case "thinking":
					logLines = append(logLines, "💭 "+e.Message)
				case "tool":
					logLines = append(logLines, "🔧 "+e.Message)
				case "text":
					logLines = append(logLines, e.Message)
				case "status":
					logLines = append(logLines, "📌 "+e.Message)
				}
			}
			logs = strings.Join(logLines, "\n")

			// output이 비어있으면 text 항목을 전부 합쳐서 가져오기
			if output == "" {
				var textParts []string
				for _, e := range entries {
					if e.Type == "text" {
						textParts = append(textParts, e.Message)
					}
				}
				if len(textParts) > 0 {
					output = strings.Join(textParts, "")
				}
			}
		}

		// instruction 파일에서 읽기 (CLI 프로세스가 기록)
		if instruction == "" {
			instrFile := filepath.Join(agent.Config.WorkDir, ".agent-instruction")
			if data, err := os.ReadFile(instrFile); err == nil {
				instruction = strings.TrimSpace(string(data))
			}
		}
	}

	// status 파일에서 최신 상태 읽기 (CLI 프로세스가 업데이트했을 수 있음)
	status := string(agent.Status)
	statusFile := filepath.Join(agent.Config.WorkDir, ".agent-status")
	if data, err := os.ReadFile(statusFile); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "" {
			status = s
		}
	}

	return &AgentDetailInfo{
		ID:           agent.Config.AgentID,
		Role:         agent.Config.Role,
		Status:       status,
		IsConsumer:   agent.Config.IsConsumer,
		Instruction:  instruction,
		Output:       output,
		Logs:         logs,
		AllowedTools: agent.Config.AllowedTools,
	}
}

// readJSONLEntries reads log entries from a JSONL file.
func readJSONLEntries(path string) ([]internal.LogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []internal.LogEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e internal.LogEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (a *App) GetAgentStatuses() []AgentStatusInfo {
	var statuses []AgentStatusInfo
	for _, agent := range a.agents {
		statuses = append(statuses, AgentStatusInfo{
			ID:         agent.Config.AgentID,
			Role:       agent.Config.Role,
			Status:     string(agent.Status),
			IsConsumer: agent.Config.IsConsumer,
		})
	}
	return statuses
}

func (a *App) GetLocks() map[string]string {
	if a.workspace == nil {
		return nil
	}
	registry := internal.NewFileLockRegistry(a.workspace.LocksDir)
	return registry.ListLocks()
}

// ── 단일 세션 모드 (Phase D+E) ──

// RunLeadSession: 팀장이 단일 Claude 세션으로 전체 워크플로를 처리합니다.
// fsnotify로 .orchestra/logs/ 를 감시하여 sub-agent JSONL 출력을 실시간 전달합니다.
func (a *App) RunLeadSession(userInput string) string {
	if a.lead == nil {
		return "프로젝트를 먼저 열어주세요."
	}

	logFn := func(msg string) {
		if len(msg) > 0 && msg[0] == '\x01' {
			runtime.EventsEmit(a.ctx, "log-append", msg[1:])
			return
		}
		evtType := "text"
		if strings.Contains(msg, "💭") || strings.Contains(msg, "🔧") || strings.Contains(msg, "📊") {
			evtType = "thinking"
		}
		runtime.EventsEmit(a.ctx, "log", LogEvent{Type: evtType, Message: msg})
	}

	// .orchestra 디렉토리만 확보 (팀 구성은 Lead 세션이 CLI로 수행)
	if a.workspace != nil {
		a.workspace.Init(nil)
	}

	// CLIPath 자동 탐색
	if a.lead.CLIPath == "" {
		a.lead.CLIPath = findCLI()
	}

	// PreToolUse 훅 설정 (프로젝트 .claude/settings.json)
	a.ensureHookSettings()

	// fsnotify 로그 감시 시작
	// 연속된 같은 타입 청크는 append로 이어붙임 (줄바꿈 방지)
	var lastAgent, lastType string
	watcher := internal.NewLogWatcher(a.workspace.LogsDir, func(entry internal.LogEntry) {
		prefix := fmt.Sprintf("[%s] ", entry.Agent)
		isContinuation := entry.Agent == lastAgent && entry.Type == lastType &&
			(entry.Type == "thinking" || entry.Type == "text")

		switch entry.Type {
		case "thinking":
			if isContinuation {
				runtime.EventsEmit(a.ctx, "log-append", entry.Message)
			} else {
				runtime.EventsEmit(a.ctx, "log", LogEvent{Type: "thinking", Message: prefix + "💭 " + entry.Message})
			}
		case "tool":
			runtime.EventsEmit(a.ctx, "log", LogEvent{Type: "thinking", Message: prefix + "🔧 " + entry.Message})
		case "text":
			if isContinuation {
				runtime.EventsEmit(a.ctx, "log-append", entry.Message)
			} else {
				runtime.EventsEmit(a.ctx, "log", LogEvent{Type: "text", Message: prefix + entry.Message})
			}
		case "status":
			runtime.EventsEmit(a.ctx, "log", LogEvent{Type: "text", Message: prefix + "📌 " + entry.Message})
			runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())
		}
		lastAgent = entry.Agent
		lastType = entry.Type
	})
	if err := watcher.Start(); err != nil {
		logFn(fmt.Sprintf("[팀장] ⚠️ 로그 감시 시작 실패: %s", err))
	}
	defer watcher.Stop()

	// permissions 감시 → GUI 승인 다이얼로그
	permDir := internal.PermissionsDir(a.workspace.Root)
	os.MkdirAll(permDir, 0755)
	permWatcher, permErr := fsnotify.NewWatcher()
	if permErr == nil {
		permWatcher.Add(permDir)
		permStop := make(chan struct{})
		go func() {
			for {
				select {
				case <-permStop:
					return
				case event, ok := <-permWatcher.Events:
					if !ok {
						return
					}
					if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) &&
						strings.Contains(event.Name, "request-") &&
						strings.HasSuffix(event.Name, ".json") {
						// 요청 파일 읽기
						data, err := os.ReadFile(event.Name)
						if err != nil {
							continue
						}
						var req internal.PermissionRequest
						if err := json.Unmarshal(data, &req); err != nil {
							continue
						}
						runtime.EventsEmit(a.ctx, "permission-request", req)
					}
				case <-permWatcher.Errors:
				}
			}
		}()
		defer func() {
			close(permStop)
			permWatcher.Close()
		}()
	}

	// team.json 변경 감시 → 사이드바 즉시 업데이트
	teamWatcher, err := fsnotify.NewWatcher()
	if err == nil {
		teamWatcher.Add(a.workspace.OrchestraDir)
		teamStop := make(chan struct{})
		go func() {
			for {
				select {
				case <-teamStop:
					return
				case event, ok := <-teamWatcher.Events:
					if !ok {
						return
					}
					if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) &&
						strings.HasSuffix(event.Name, "team.json") {
						if plans := a.workspace.LoadRolePlans(); len(plans) > 0 {
							a.rolePlans = plans
							a.buildTeamFromPlans(plans)
							runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())
						}
					}
				case <-teamWatcher.Errors:
				}
			}
		}()
		defer func() {
			close(teamStop)
			teamWatcher.Close()
		}()
	}

	// 단일 세션 실행
	result := a.lead.RunLeadSession(userInput, logFn)

	// 세션 완료 후 팀 리로드 (Lead가 team set으로 팀을 생성했을 수 있음)
	if plans := a.workspace.LoadRolePlans(); len(plans) > 0 && len(plans) != len(a.rolePlans) {
		a.rolePlans = plans
		a.buildTeamFromPlans(plans)
	}
	runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())

	return result
}

// ── 프로젝트 디렉토리 선택 ──

func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "프로젝트 디렉토리 선택",
	})
	return dir, err
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// ── 세션 취소 ──

// CancelSession: 실행 중인 팀장 세션을 강제 중단합니다.
func (a *App) CancelSession() {
	if a.lead != nil {
		a.lead.Cancel()
	}
}

// ── 권한 승인/거부 ──

// RespondPermission: 프론트엔드에서 allow/disallow 클릭 시 호출
func (a *App) RespondPermission(id string, allowed bool) error {
	if a.workspace == nil {
		return fmt.Errorf("프로젝트가 열려있지 않습니다")
	}
	permDir := internal.PermissionsDir(a.workspace.Root)
	return internal.WriteResponse(permDir, &internal.PermissionResponse{
		ID:      id,
		Allowed: allowed,
	})
}

// ensureHookSettings writes PreToolUse hook config to project .claude/settings.json
func (a *App) ensureHookSettings() {
	if a.workspace == nil {
		return
	}
	cliPath := a.lead.CLIPath
	if cliPath == "" {
		cliPath = findCLI()
	}

	settingsDir := filepath.Join(a.workspace.Root, ".claude")
	settingsFile := filepath.Join(settingsDir, "settings.json")

	// 기존 설정 읽기
	var settings map[string]interface{}
	if data, err := os.ReadFile(settingsFile); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}

	// 훅 설정 구성
	hookCommand := cliPath + " hook pretooluse"
	expectedHook := map[string]interface{}{
		"type":    "command",
		"command": hookCommand,
		"timeout": 300,
	}

	hookEntry := map[string]interface{}{
		"matcher": ".*",
		"hooks":   []interface{}{expectedHook},
	}

	settings["hooks"] = map[string]interface{}{
		"PreToolUse": []interface{}{hookEntry},
	}

	os.MkdirAll(settingsDir, 0755)
	data, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(settingsFile, data, 0644)
}

// findCLI searches for the claudestra binary in common locations.
func findCLI() string {
	// 1. PATH에서 찾기
	if path, err := exec.LookPath("claudestra"); err == nil {
		return path
	}
	// 2. ~/go/bin/ (go install 위치)
	if home, err := os.UserHomeDir(); err == nil {
		gobin := filepath.Join(home, "go", "bin", "claudestra")
		if _, err := os.Stat(gobin); err == nil {
			return gobin
		}
	}
	// 3. 실행파일 옆
	if self, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(self), "claudestra")
		if _, err := os.Stat(beside); err == nil {
			return beside
		}
	}
	// 폴백: PATH에 있다고 가정
	return "claudestra"
}
