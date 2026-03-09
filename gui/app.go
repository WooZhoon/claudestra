package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gui/internal"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LogEvent is a structured log entry sent to the frontend.
type LogEvent struct {
	Type    string `json:"type"`    // "text", "thinking", "status"
	Message string `json:"message"`
}

// ProposalInfo is returned to the GUI for user review before execution.
type ProposalInfo struct {
	UserInput string           `json:"userInput"`
	TeamPlans []TeamPlanInfo   `json:"teamPlans"` // nil if team already exists
	Steps     []StepInfo       `json:"steps"`
	Contract  string           `json:"contract"`
	NeedTeam  bool             `json:"needTeam"` // true if team needs to be created
}

type TeamPlanInfo struct {
	Role        string `json:"role"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

type StepInfo struct {
	StepNum int        `json:"step"`
	Title   string     `json:"title"`
	Tasks   []TaskInfo `json:"tasks"`
}

type TaskInfo struct {
	AgentID     string `json:"agentId"`
	Instruction string `json:"instruction"`
}

type App struct {
	ctx             context.Context
	workspace       *internal.Workspace
	lead            *internal.LeadAgent
	agents          map[string]*internal.Agent
	rolePlans       []internal.RolePlan
	pendingProposal *ProposalInfo // 승인 대기 중인 계획
	pendingPlans    []internal.RolePlan
	pendingSteps    []internal.Step
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

// ── 사용자 요청 처리 (2단계: 계획 → 실행) ──

// PlanRequest: 팀장이 요구사항을 분석하고 계획을 제안합니다. (실행하지 않음)
func (a *App) PlanRequest(userInput string) (*ProposalInfo, error) {
	if a.lead == nil {
		return nil, fmt.Errorf("프로젝트를 먼저 열어주세요.")
	}

	logFn := func(msg string) {
		// \x01 prefix = append to last log entry
		if len(msg) > 0 && msg[0] == '\x01' {
			runtime.EventsEmit(a.ctx, "log-append", msg[1:])
			return
		}
		evtType := "text"
		if strings.Contains(msg, "💭") || strings.Contains(msg, "🔧") {
			evtType = "thinking"
		}
		runtime.EventsEmit(a.ctx, "log", LogEvent{Type: evtType, Message: msg})
	}

	// streaming 활성화: 내부 callClaude가 모두 streaming으로 동작
	a.lead.SetLogFn(logFn)
	defer a.lead.SetLogFn(nil)

	if len(a.agents) == 0 {
		// 팀이 없는 상태 — 먼저 개발 태스크인지 판단
		logFn("[팀장] 🤔 개발 태스크인지 판단 중...")
		isDev := a.lead.IsDevTask(userInput)
		if !isDev {
			logFn("[팀장] 💬 직접 응답합니다.")
			reply := a.lead.DirectReply(userInput)
			a.lead.SaveDirectReply(userInput, reply)
			return nil, fmt.Errorf("DIRECT_REPLY:%s", reply)
		}

		// 개발 태스크 → 팀 구성
		logFn("\n[팀장] 🏗️ 팀을 구성합니다...")
		plans := a.lead.PlanTeam(userInput)
		a.pendingPlans = plans
		logFn(fmt.Sprintf("[팀장] ✅ 팀 구성 완료! %d명", len(plans)))

		var roles []string
		for _, p := range plans {
			roles = append(roles, p.Role)
		}
		a.workspace.Init(roles)
		a.buildTeamFromPlans(plans)
		// buildTeamFromPlans가 a.lead를 새로 생성하므로 logFn을 다시 설정
		a.lead.SetLogFn(logFn)
	}

	// 2. 실행 계획 수립
	logFn("\n[팀장] 📋 실행 계획 수립 중...")
	steps := a.lead.Decompose(userInput)
	a.pendingSteps = steps

	if len(steps) == 0 {
		logFn("[팀장] 💬 개발 태스크가 아닙니다. 팀장이 직접 응답합니다.")
		a.pendingProposal = nil
		reply := a.lead.DirectReply(userInput)
		a.lead.SaveDirectReply(userInput, reply)
		return nil, fmt.Errorf("DIRECT_REPLY:%s", reply)
	}

	// 3. Proposal 구성
	proposal := &ProposalInfo{UserInput: userInput}

	if a.pendingPlans != nil {
		proposal.NeedTeam = true
		for _, p := range a.pendingPlans {
			proposal.TeamPlans = append(proposal.TeamPlans, TeamPlanInfo{
				Role:        p.Role,
				Description: p.Description,
				Type:        p.Type,
			})
		}
	}

	for _, step := range steps {
		si := StepInfo{StepNum: step.StepNum, Title: step.Title}
		for _, t := range step.Tasks {
			si.Tasks = append(si.Tasks, TaskInfo{AgentID: t.AgentID, Instruction: t.Instruction})
		}
		proposal.Steps = append(proposal.Steps, si)
	}

	logFn(fmt.Sprintf("[팀장] ✅ 계획 수립 완료! %d단계", len(steps)))

	// 3. 계약서 미리 생성
	logFn("\n[팀장] 📜 계약서 작성 중...")
	contract := a.lead.GenerateContract(userInput, steps)
	proposal.Contract = contract

	a.pendingProposal = proposal
	logFn("[팀장] 계획 수립 완료. 검토 후 실행을 승인해주세요.")
	return proposal, nil
}

// ExecutePlan: 사용자가 승인한 계획을 실행합니다.
func (a *App) ExecutePlan() string {
	if a.pendingProposal == nil {
		return "실행할 계획이 없습니다."
	}

	logFn := func(msg string) {
		// \x01 prefix = append to last log entry
		if len(msg) > 0 && msg[0] == '\x01' {
			runtime.EventsEmit(a.ctx, "log-append", msg[1:])
			return
		}
		evtType := "text"
		if strings.Contains(msg, "💭") || strings.Contains(msg, "🔧") {
			evtType = "thinking"
		}
		runtime.EventsEmit(a.ctx, "log", LogEvent{Type: evtType, Message: msg})
	}

	proposal := a.pendingProposal
	steps := a.pendingSteps

	// 팀 구성 확정 (아직 저장 안 됐으면)
	if proposal.NeedTeam && a.pendingPlans != nil {
		a.workspace.SaveRolePlans(a.pendingPlans)
		a.rolePlans = a.pendingPlans
		runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())
	}

	// 실행
	result := a.lead.ExecuteApprovedPlan(proposal.UserInput, steps, proposal.Contract, logFn)

	a.pendingProposal = nil
	a.pendingPlans = nil
	a.pendingSteps = nil

	// 실행 후 상태 갱신
	runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())

	return result
}

// SubmitRequest: 계획 없이 바로 실행 (레거시 호환)
func (a *App) SubmitRequest(userInput string) string {
	if a.lead == nil {
		return "프로젝트를 먼저 열어주세요."
	}

	logFn := func(msg string) {
		// \x01 prefix = append to last log entry
		if len(msg) > 0 && msg[0] == '\x01' {
			runtime.EventsEmit(a.ctx, "log-append", msg[1:])
			return
		}
		evtType := "text"
		if strings.Contains(msg, "💭") || strings.Contains(msg, "🔧") {
			evtType = "thinking"
		}
		runtime.EventsEmit(a.ctx, "log", LogEvent{Type: evtType, Message: msg})
	}

	a.lead.SetLogFn(logFn)
	defer a.lead.SetLogFn(nil)

	if len(a.agents) == 0 {
		logFn("[팀장] 요구사항을 분석하여 팀을 구성합니다...")
		plans := a.lead.PlanTeam(userInput)
		var roles []string
		for _, p := range plans {
			roles = append(roles, p.Role)
		}
		a.workspace.Init(roles)
		a.workspace.SaveRolePlans(plans)
		a.rolePlans = plans
		a.buildTeamFromPlans(plans)
		// buildTeamFromPlans가 a.lead를 새로 생성하므로 logFn을 다시 설정
		a.lead.SetLogFn(logFn)
		runtime.EventsEmit(a.ctx, "team-updated", a.GetAgentStatuses())
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

// ── 프로젝트 디렉토리 선택 ──

func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "프로젝트 디렉토리 선택",
	})
	return dir, err
}

func truncate(s string, maxLen int) string {
	// 줄바꿈 제거 후 truncate
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
