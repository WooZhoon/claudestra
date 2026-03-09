package main

import (
	"context"
	"fmt"

	"gui/internal"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	workspace *internal.Workspace
	lead      *internal.LeadAgent
	agents    map[string]*internal.Agent
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

func (a *App) InitProject(projectDir string, roles []string) error {
	a.workspace = internal.NewWorkspace(projectDir)
	if err := a.workspace.Init(roles); err != nil {
		return err
	}
	a.buildTeam(roles)
	return nil
}

func (a *App) OpenProject(projectDir string) error {
	a.workspace = internal.NewWorkspace(projectDir)
	config, err := a.workspace.LoadConfig()
	if err != nil {
		return fmt.Errorf("프로젝트를 열 수 없습니다: %w", err)
	}
	a.buildTeam(config.Agents)
	return nil
}

func (a *App) buildTeam(roles []string) {
	lockRegistry := internal.NewFileLockRegistry(a.workspace.LocksDir)
	a.lead = internal.NewLeadAgent(a.workspace.Root)
	a.agents = make(map[string]*internal.Agent)

	for _, role := range roles {
		idea := a.workspace.LoadIdea(role)
		agentDir := a.workspace.Root + "/" + role

		var readRefs []string
		if a.workspace.IsConsumer(role) {
			readRefs = a.workspace.GetProducerDirs(role)
		}

		allowedTools := internal.RoleTools[role]

		config := internal.AgentConfig{
			AgentID:      role,
			Role:         role,
			Idea:         idea,
			WorkDir:      agentDir,
			ReadRefs:     readRefs,
			AllowedTools: allowedTools,
		}
		agent := internal.NewAgent(config, lockRegistry)
		a.agents[role] = agent
		a.lead.AddAgent(agent)
	}
}

// ── 사용자 요청 처리 ──

func (a *App) SubmitRequest(userInput string) string {
	if a.lead == nil {
		return "프로젝트를 먼저 열어주세요."
	}

	logFn := func(msg string) {
		// GUI로 실시간 로그 전송
		runtime.EventsEmit(a.ctx, "log", msg)
	}

	return a.lead.Process(userInput, logFn)
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

func (a *App) GetAgentStatuses() []AgentStatusInfo {
	var statuses []AgentStatusInfo
	for _, agent := range a.agents {
		statuses = append(statuses, AgentStatusInfo{
			ID:         agent.Config.AgentID,
			Role:       agent.Config.Role,
			Status:     string(agent.Status),
			IsConsumer: a.workspace.IsConsumer(agent.Config.Role),
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

// ── 프로젝트 디렉토리 선택 ──

func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "프로젝트 디렉토리 선택",
	})
	return dir, err
}
