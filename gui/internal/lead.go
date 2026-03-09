package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// ── 세션 메모리 ──

type OpenIssue struct {
	ID          string `json:"id"`
	FoundBy     string `json:"found_by"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	File        string `json:"file"`
	Status      string `json:"status"` // "open" or "resolved"
}

type ConversationTurn struct {
	Role    string `json:"role"` // "user" or "lead"
	Content string `json:"content"`
}

type Session struct {
	ProjectSummary      string             `json:"project_summary"`
	CompletedTasks      []string           `json:"completed_tasks"`
	OpenIssues          []OpenIssue        `json:"open_issues"`
	RecentConversations []ConversationTurn `json:"recent_conversations"`
}

// ── LeadAgent ──

// LogFunc is called for real-time log streaming to GUI
type LogFunc func(msg string)

type LeadAgent struct {
	WorkDir     string
	CLIPath     string // claudestra CLI 바이너리 경로
	Agents      map[string]*Agent
	session     *Session
	activeLogFn LogFunc // 현재 활성화된 로그 콜백 (streaming용)
	activeCmd   *exec.Cmd
	cmdMu       sync.Mutex
}

// RolePlan describes a dynamically planned agent role.
type RolePlan struct {
	Role        string `json:"role"`
	Description string `json:"description"`
	Type        string `json:"type"`      // "producer" or "consumer"
	Directory   string `json:"directory"` // working directory name
}

func NewLeadAgent(workDir string) *LeadAgent {
	os.MkdirAll(workDir, 0755)
	l := &LeadAgent{
		WorkDir: workDir,
		Agents:  make(map[string]*Agent),
	}
	l.session = l.loadSession()
	return l
}

func (l *LeadAgent) sessionPath() string {
	return filepath.Join(l.WorkDir, ".orchestra", "session.json")
}

func (l *LeadAgent) loadSession() *Session {
	data, err := os.ReadFile(l.sessionPath())
	if err != nil {
		return &Session{}
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return &Session{}
	}
	return &s
}

func (l *LeadAgent) buildSessionBlock() string {
	s := l.session
	if s.ProjectSummary == "" && len(s.CompletedTasks) == 0 && len(s.OpenIssues) == 0 {
		return ""
	}

	var parts []string

	if s.ProjectSummary != "" {
		parts = append(parts, fmt.Sprintf("[프로젝트 현재 상태]\n%s", s.ProjectSummary))
	}

	if len(s.CompletedTasks) > 0 {
		// 최근 10개만
		tasks := s.CompletedTasks
		if len(tasks) > 10 {
			tasks = tasks[len(tasks)-10:]
		}
		parts = append(parts, fmt.Sprintf("[완료된 작업]\n- %s", strings.Join(tasks, "\n- ")))
	}

	var openIssues []string
	for _, issue := range s.OpenIssues {
		if issue.Status == "open" {
			openIssues = append(openIssues, fmt.Sprintf("[%s] %s: %s (%s)", issue.Severity, issue.ID, issue.Description, issue.File))
		}
	}
	if len(openIssues) > 0 {
		parts = append(parts, fmt.Sprintf("[미해결 이슈]\n- %s", strings.Join(openIssues, "\n- ")))
	}

	if len(s.RecentConversations) > 0 {
		var convLines []string
		for _, c := range s.RecentConversations {
			convLines = append(convLines, fmt.Sprintf("%s: %s", c.Role, c.Content))
		}
		parts = append(parts, fmt.Sprintf("[최근 대화]\n%s", strings.Join(convLines, "\n")))
	}

	return "\n" + strings.Join(parts, "\n\n") + "\n"
}

func (l *LeadAgent) AddAgent(agent *Agent) {
	l.Agents[agent.Config.AgentID] = agent
}

// ── Phase D: 단일 세션 모드 ──

// RunLeadSession은 단일 Claude 세션으로 전체 워크플로를 처리합니다.
// Lead 1회 + sub-agent N회로 Claude 호출을 최소화합니다.
// Lead는 claudestra CLI를 Bash 도구로 호출하여 팀 관리, 계약서, 태스크 배분을 수행합니다.
func (l *LeadAgent) RunLeadSession(userInput string, logFn LogFunc) string {
	if logFn == nil {
		logFn = func(msg string) { fmt.Println(msg) }
	}
	l.activeLogFn = logFn
	defer func() { l.activeLogFn = nil }()

	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn(fmt.Sprintf("[팀장] 단일 세션 시작: %s", userInput))
	logFn(strings.Repeat("=", 60))

	prompt := l.buildLeadSessionPrompt(userInput)

	args := []string{"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--dangerously-skip-permissions",
		"--allowedTools", "Bash,Read,Glob,Grep",
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = l.WorkDir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = filterEnv(os.Environ(), "CLAUDECODE")
	// 프로세스 그룹 설정 — Cancel 시 자식 프로세스도 함께 종료
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logFn("[팀장] ❌ 파이프 오류")
		return fmt.Sprintf("PIPE ERROR: %s", err)
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		logFn("[팀장] ❌ 시작 오류")
		return fmt.Sprintf("START ERROR: %s", err)
	}

	l.cmdMu.Lock()
	l.activeCmd = cmd
	l.cmdMu.Unlock()
	defer func() {
		l.cmdMu.Lock()
		l.activeCmd = nil
		l.cmdMu.Unlock()
	}()

	var fullResult string
	textStarted := false
	thinkStarted := false
	ParseStream(stdout, StreamCallbacks{
		OnText: func(text string) {
			thinkStarted = false
			if !textStarted {
				logFn(fmt.Sprintf("[팀장] %s", text))
				textStarted = true
			} else {
				logFn("\x01" + text)
			}
		},
		OnThinking: func(text string) {
			textStarted = false
			if !thinkStarted {
				logFn("  💭 " + text)
				thinkStarted = true
			} else {
				logFn("\x01" + text)
			}
		},
		OnToolUse: func(toolName string, input string) {
			textStarted = false
			thinkStarted = false
			msg := "  🔧 " + toolName
			if input != "" {
				msg += ": " + input
			}
			logFn(msg)
		},
		OnResult: func(result string) {
			textStarted = false
			thinkStarted = false
			fullResult = result
		},
	})

	cmd.Wait()
	result := strings.TrimSpace(fullResult)

	if result == "" {
		logFn("\n[팀장] ⛔ 세션 중단됨")
		return "세션이 중단되었습니다."
	}

	logFn(fmt.Sprintf("\n[팀장] ✅ 세션 완료"))

	return result
}

// Cancel kills the running lead session process and all its children.
func (l *LeadAgent) Cancel() {
	l.cmdMu.Lock()
	cmd := l.activeCmd
	l.cmdMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	// 프로세스 그룹 전체 종료 (lead Claude + 자식 claudestra assign + sub-agent Claude)
	syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

func (l *LeadAgent) buildLeadSessionPrompt(userInput string) string {
	cliCmd := "claudestra"
	if l.CLIPath != "" {
		cliCmd = l.CLIPath
	}

	sessionBlock := l.buildSessionBlock()

	tmpl := `당신은 소프트웨어 개발 팀의 팀장 AI입니다.
Bash 도구를 통해 claudestra CLI로 팀을 관리하고 작업을 수행합니다.

[사용 가능한 CLI 명령어]
{CLI} team                          팀원 목록 (JSON, 비어있으면 [])
{CLI} team set '<json>'             팀 구성 설정 (팀원 생성/변경)
{CLI} status                        팀원 상태 확인
{CLI} session get                   세션 메모리 조회 (JSON)
{CLI} session update '<json>'       세션 메모리 갱신
{CLI} issues                        미해결 이슈 목록
{CLI} contract get                  계약서 조회
{CLI} contract set '<yaml>'         계약서 설정
{CLI} idea <agent>                  에이전트 이데아(역할 설명) 조회
{CLI} output <agent>                에이전트 최근 출력 조회
{CLI} assign <agent> '<instruction>'         동기 작업 지시 (완료까지 대기)
{CLI} assign --async <agent> '<instruction>' 비동기 작업 지시 (job-id 반환)

[team set JSON 형식]
[{"role":"영문_snake_case","description":"한국어 역할 설명","type":"producer|consumer","directory":"영문_디렉토리"}]
- producer: 코드/파일을 작성하는 팀원
- consumer: 다른 팀원의 결과물을 읽기 전용으로 분석하는 팀원 (리뷰어, QA 등)

[작업 흐름]
1. {CLI} team 으로 팀 구성을 확인하세요
2. 팀이 비어있으면 ([] 반환):
   a. 프로젝트 구조를 파악하세요 (파일 목록, 기존 코드 등)
   b. 사용자 요청에 필요한 역할을 분석하세요
   c. {CLI} team set 으로 적절한 팀을 구성하세요
3. {CLI} session get 으로 프로젝트 컨텍스트를 파악하세요
4. 사용자 요청을 분석하세요
5. 개발 태스크가 아니면 (인사, 질문, 잡담) 직접 한국어로 응답하세요
6. 개발 태스크이면:
   a. 필요하면 {CLI} contract set 으로 인터페이스 계약서를 설정하세요
   b. {CLI} assign <agent> '<instruction>' 으로 각 팀원에게 작업을 지시하세요
   c. 의존성이 없는 작업은 --async로 병렬 실행할 수 있습니다
   d. 모든 작업 완료 후 결과를 취합하여 최종 보고서를 작성하세요
   e. {CLI} session update 로 세션 메모리를 갱신하세요

[중요 규칙]
- 한국어로 응답하세요
- 프로젝트의 실제 도메인에 맞는 역할을 구성하세요 (웹이 아니면 backend/frontend 쓰지 마세요)
- assign의 instruction은 간결하고 구체적으로 작성하세요
- 작업 지시 시 핵심 구조와 주요 파일만 구현하도록 안내하세요
- 최종 보고서에는 완료 작업 요약, 각 팀원 수행 내용, 결과 평가, 다음 단계 제안을 포함하세요
{SESSION}
[사용자 요청]
{INPUT}`

	result := strings.ReplaceAll(tmpl, "{CLI}", cliCmd)
	result = strings.ReplaceAll(result, "{SESSION}", sessionBlock)
	result = strings.ReplaceAll(result, "{INPUT}", userInput)
	return result
}
