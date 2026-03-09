package main

import (
	"context"
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
	AllowedTools []string `json:"allowedTools"`
}

func (a *App) GetAgentDetail(agentID string) *AgentDetailInfo {
	agent, ok := a.agents[agentID]
	if !ok {
		return nil
	}
	return &AgentDetailInfo{
		ID:           agent.Config.AgentID,
		Role:         agent.Config.Role,
		Status:       string(agent.Status),
		IsConsumer:   agent.Config.IsConsumer,
		Instruction:  agent.LastInstruction,
		Output:       agent.Output,
		AllowedTools: agent.Config.AllowedTools,
	}
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
