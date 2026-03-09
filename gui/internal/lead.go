package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// extractJSON extracts JSON content from text that may contain analysis + ```json block.
// Falls back to finding first [ or { if no fenced block.
func extractJSON(raw string) string {
	// 1. ```json 블록에서 추출
	re := regexp.MustCompile("(?s)```json\\s*\n(.*?)\\s*```")
	if m := re.FindStringSubmatch(raw); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// 2. ``` 블록 (언어 미지정)
	re2 := regexp.MustCompile("(?s)```\\s*\n(\\[.*?\\])\\s*```")
	if m := re2.FindStringSubmatch(raw); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// 3. 폴백: 첫 번째 [ ... ] 또는 { ... } 찾기
	if idx := strings.Index(raw, "["); idx >= 0 {
		return strings.TrimSpace(raw[idx:])
	}
	if idx := strings.Index(raw, "{"); idx >= 0 {
		return strings.TrimSpace(raw[idx:])
	}
	return raw
}

// extractYAML extracts YAML content from text that may contain analysis + ```yaml block.
func extractYAML(raw string) string {
	re := regexp.MustCompile("(?s)```ya?ml\\s*\n(.*?)\\s*```")
	if m := re.FindStringSubmatch(raw); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// 폴백: ``` 블록 제거
	re2 := regexp.MustCompile("(?s)```\\w*\\s*|\\s*```")
	cleaned := strings.TrimSpace(re2.ReplaceAllString(raw, ""))
	if cleaned != "" {
		return cleaned
	}
	return raw
}

const leadIdea = `당신은 소프트웨어 개발 팀의 팀장 AI입니다.
당신의 역할은:
1. 사용자의 요구사항을 분석합니다.
2. 각 팀원의 역할에 맞게 태스크를 분해합니다.
3. 각 태스크 간의 의존관계(depends_on)를 명시합니다.

중요: 개발 태스크가 아닌 경우(인사, 잡담, 질문 등)에는 빈 배열 []을 반환하세요.
반드시 JSON 형식으로만 응답하세요. 다른 텍스트는 포함하지 마세요.`

// ── 태스크 관련 타입 ──

type RawTask struct {
	ID          string   `json:"id"`
	AgentID     string   `json:"agent_id"`
	Instruction string   `json:"instruction"`
	DependsOn   []string `json:"depends_on"`
}

type StepTask struct {
	AgentID     string `json:"agent_id"`
	Instruction string `json:"instruction"`
}

type Step struct {
	StepNum int        `json:"step"`
	Title   string     `json:"title"`
	Tasks   []StepTask `json:"tasks"`
}

// ── 에이전트 정보 ──

type AgentInfo struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

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

type LeadAgent struct {
	WorkDir     string
	Agents      map[string]*Agent
	session     *Session
	activeLogFn LogFunc // 현재 활성화된 로그 콜백 (streaming용)
	mu          sync.Mutex
}

// SetLogFn sets the active log callback for streaming output.
// Call with nil to disable streaming.
func (l *LeadAgent) SetLogFn(fn LogFunc) {
	l.activeLogFn = fn
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

func (l *LeadAgent) saveSession() {
	dir := filepath.Dir(l.sessionPath())
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(l.session, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(l.sessionPath(), data, 0644)
}

func (l *LeadAgent) addConversation(role, content string) {
	truncated := content
	if len(truncated) > 200 {
		truncated = truncated[:200] + "..."
	}
	l.session.RecentConversations = append(l.session.RecentConversations, ConversationTurn{
		Role:    role,
		Content: truncated,
	})
	// 최근 10턴만 유지
	if len(l.session.RecentConversations) > 10 {
		l.session.RecentConversations = l.session.RecentConversations[len(l.session.RecentConversations)-10:]
	}
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
	// stdout log (CLI mode only, GUI uses logFn)
}

func (l *LeadAgent) listAgents() []AgentInfo {
	var agents []AgentInfo
	for _, a := range l.Agents {
		agents = append(agents, AgentInfo{
			ID:     a.Config.AgentID,
			Role:   a.Config.Role,
			Status: string(a.Status),
		})
	}
	return agents
}

// ── 팀 자동 구성 ──

// RolePlan describes a dynamically planned agent role.
type RolePlan struct {
	Role        string `json:"role"`
	Description string `json:"description"`
	Type        string `json:"type"`      // "producer" or "consumer"
	Directory   string `json:"directory"` // working directory name
}

// PlanTeam asks Claude to analyze the request and design a team with appropriate roles.
func (l *LeadAgent) PlanTeam(userInput string) []RolePlan {
	contextBlock := l.buildSessionBlock()

	prompt := fmt.Sprintf(`당신은 프로젝트 팀의 팀장입니다.
사용자의 요구사항과 프로젝트 컨텍스트를 분석하여 최적의 팀을 구성하세요.
%s

먼저 프로젝트를 분석하세요:
1. 이 프로젝트가 어떤 도메인/기술 스택인지 파악
2. 사용자의 요구사항에 어떤 전문가가 필요한지 설명
3. 각 팀원의 역할과 협업 방식을 설명

분석을 마친 후, 아래 형식의 JSON을 ` + "```json" + ` 블록으로 출력하세요:
[
  {"role": "영문_snake_case", "description": "한국어 설명 (3~5줄)\n담당 디렉토리: ./directory/", "type": "producer|consumer", "directory": "영문"}
]

핵심 규칙:
- 프로젝트의 실제 도메인에 맞는 역할을 정의하세요. 웹 프로젝트가 아니면 backend/frontend를 쓰지 마세요.
  예: 로봇/비전 → vision_dev, calibration_eng, build_qa 등
  예: 모바일 앱 → ios_dev, android_dev, api_server 등
  예: 데이터 파이프라인 → data_engineer, ml_engineer, infra 등
- 요구사항에 필요한 역할만 포함하세요. 최소 1명, 최대 8명.
- producer: 실제 코드나 파일을 작성하는 팀원
- consumer: 다른 팀원의 결과물을 읽기 전용으로 분석하는 팀원 (리뷰어, QA 등)
- consumer의 description에는 "읽기 전용으로 다른 팀원의 코드를 참조합니다"를 포함하세요.

[사용자 요구사항]
%s`, contextBlock, userInput)

	raw := l.callClaude(prompt, 60)
	if raw == "" {
		return defaultTeam()
	}

	jsonStr := extractJSON(raw)
	var plans []RolePlan
	if err := json.Unmarshal([]byte(jsonStr), &plans); err != nil {
		return defaultTeam()
	}

	// 유효성 검증
	var valid []RolePlan
	for _, p := range plans {
		if p.Role == "" || p.Description == "" || p.Directory == "" {
			continue
		}
		if p.Type != "producer" && p.Type != "consumer" {
			p.Type = "producer"
		}
		valid = append(valid, p)
	}
	if len(valid) == 0 {
		return defaultTeam()
	}
	return valid
}

func defaultTeam() []RolePlan {
	return []RolePlan{
		{Role: "developer", Description: "소프트웨어 개발 전문가입니다.\n담당 디렉토리: ./src/\n프로젝트의 핵심 코드를 작성합니다.", Type: "producer", Directory: "src"},
		{Role: "reviewer", Description: "코드 리뷰 전문가입니다.\n읽기 전용으로 다른 팀원의 코드를 참조합니다.\n담당 디렉토리: ./reviewer/", Type: "consumer", Directory: "reviewer"},
	}
}

// IsDevTask asks Claude whether the user input is a development task or not.
// Used when no team exists yet to avoid unnecessary team creation.
func (l *LeadAgent) IsDevTask(userInput string) bool {
	contextBlock := l.buildSessionBlock()

	prompt := fmt.Sprintf(`사용자의 메시지가 소프트웨어 개발/구현/설계 등 실제 코딩 작업이 필요한 요청인지 판단하세요.

먼저 판단 근거를 간단히 설명하세요 (1~2문장).
그 다음 마지막 줄에 "YES" 또는 "NO"만 적으세요.

규칙:
- 코드 작성, 구현, 설계, 버그 수정, 프로젝트 생성 등 → YES
- 파일 읽기, 요약, 질문, 인사, 잡담 등 → NO
- 세션에 미해결 이슈가 있고 "고쳐줘", "수정해", "이어서" 등이면 → YES
%s
[사용자 메시지]
%s`, contextBlock, userInput)

	result := l.callClaude(prompt, 30)
	// 마지막 줄에서 YES/NO 추출
	lines := strings.Split(strings.TrimSpace(result), "\n")
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	return strings.ToUpper(lastLine) == "YES"
}

// ── 핵심: 사용자 입력 처리 ──

// LogFunc is called for real-time log streaming to GUI
type LogFunc func(msg string)

func (l *LeadAgent) Process(userInput string, logFn LogFunc) string {
	if logFn == nil {
		logFn = func(msg string) { fmt.Println(msg) }
	}
	l.activeLogFn = logFn
	defer func() { l.activeLogFn = nil }()

	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn(fmt.Sprintf("[팀장] 사용자 입력 수신: %s", userInput))
	logFn(strings.Repeat("=", 60))

	// 1. 계획 수립
	logFn("\n[팀장] 📋 실행 계획 수립 중...")
	plan := l.Decompose(userInput)
	if plan == nil {
		return "계획 수립에 실패했습니다."
	}
	if len(plan) == 0 {
		logFn("[팀장] 💬 개발 태스크가 아닙니다. 팀장이 직접 응답합니다.")
		reply := l.DirectReply(userInput)
		l.addConversation("user", userInput)
		l.addConversation("lead", reply)
		l.saveSession()
		return reply
	}

	// 2. 계약서 생성
	logFn("\n[팀장] 📜 인터페이스 계약서 작성 중...")
	contract := l.GenerateContract(userInput, plan)

	// 3. 계획 표시
	l.printPlan(plan, logFn)
	if contract != "" {
		l.printContract(contract, logFn)
	}

	// 4. 계약서 주입
	if contract != "" {
		l.saveContract(contract)
		for _, agent := range l.Agents {
			agent.Config.Contract = contract
		}
	}

	// 5. 웨이브별 실행
	allResults := make(map[string]string)
	for _, step := range plan {
		logFn(fmt.Sprintf("\n%s", strings.Repeat("─", 60)))
		logFn(fmt.Sprintf("📌 %d단계: %s", step.StepNum, step.Title))
		logFn(strings.Repeat("─", 60))

		results := l.executeStep(step.Tasks, logFn)
		for k, v := range results {
			allResults[k] = v
		}

		done := 0
		for aid := range results {
			if a, ok := l.Agents[aid]; ok && a.Status == StatusDone {
				done++
			}
		}
		logFn(fmt.Sprintf("\n  ✅ %d단계 완료 (%d/%d 성공)", step.StepNum, done, len(step.Tasks)))
	}

	// 6. 최종 보고
	if len(allResults) > 0 {
		logFn("\n[팀장] 📝 최종 보고 작성 중...")
		return l.summarize(userInput, allResults, logFn)
	}
	return "실행된 태스크가 없습니다."
}

// ExecuteApprovedPlan: 사용자가 승인한 계획을 실행합니다.
func (l *LeadAgent) ExecuteApprovedPlan(userInput string, plan []Step, contract string, logFn LogFunc) string {
	if logFn == nil {
		logFn = func(msg string) { fmt.Println(msg) }
	}
	l.activeLogFn = logFn
	defer func() { l.activeLogFn = nil }()

	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn(fmt.Sprintf("[팀장] 승인된 계획 실행 시작: %s", userInput))
	logFn(strings.Repeat("=", 60))

	// 계획 표시
	l.printPlan(plan, logFn)

	// 계약서 주입
	if contract != "" {
		l.printContract(contract, logFn)
		l.saveContract(contract)
		for _, agent := range l.Agents {
			agent.Config.Contract = contract
		}
	}

	// 웨이브별 실행
	allResults := make(map[string]string)
	for _, step := range plan {
		logFn(fmt.Sprintf("\n%s", strings.Repeat("─", 60)))
		logFn(fmt.Sprintf("📌 %d단계: %s", step.StepNum, step.Title))
		logFn(strings.Repeat("─", 60))

		results := l.executeStep(step.Tasks, logFn)
		for k, v := range results {
			allResults[k] = v
		}

		done := 0
		for aid := range results {
			if a, ok := l.Agents[aid]; ok && a.Status == StatusDone {
				done++
			}
		}
		logFn(fmt.Sprintf("\n  ✅ %d단계 완료 (%d/%d 성공)", step.StepNum, done, len(step.Tasks)))
	}

	// 최종 보고
	if len(allResults) > 0 {
		logFn("\n[팀장] 📝 최종 보고 작성 중...")
		return l.summarize(userInput, allResults, logFn)
	}
	return "실행된 태스크가 없습니다."
}

func (l *LeadAgent) printPlan(plan []Step, logFn LogFunc) {
	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn("📋 실행 계획")
	logFn(strings.Repeat("=", 60))
	for _, step := range plan {
		logFn(fmt.Sprintf("\n  📌 %d단계: %s", step.StepNum, step.Title))
		for _, t := range step.Tasks {
			desc := t.Instruction
			if len(desc) > 70 {
				desc = desc[:70]
			}
			logFn(fmt.Sprintf("     [%s] %s...", t.AgentID, desc))
		}
	}
	logFn(strings.Repeat("=", 60))
}

func (l *LeadAgent) printContract(contract string, logFn LogFunc) {
	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn("📜 인터페이스 계약서")
	logFn(strings.Repeat("=", 60))
	logFn(contract)
	logFn(strings.Repeat("=", 60))
}

func (l *LeadAgent) saveContract(contract string) {
	dir := filepath.Join(l.WorkDir, ".orchestra", "contracts")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "contract.yaml"), []byte(contract), 0644)
}

// ── 세션 갱신 ──

// updateSession은 작업 완료 후 세션을 갱신합니다.
func (l *LeadAgent) updateSession(userInput string, results map[string]string, report string) {
	// 1. 대화 기록 추가
	l.addConversation("user", userInput)

	leadSummary := report
	if len(leadSummary) > 200 {
		leadSummary = leadSummary[:200] + "..."
	}
	l.addConversation("lead", leadSummary)

	// 2. 완료된 태스크 추가
	for aid, output := range results {
		agent, ok := l.Agents[aid]
		if !ok {
			continue
		}
		taskSummary := output
		if len(taskSummary) > 100 {
			taskSummary = taskSummary[:100] + "..."
		}
		l.session.CompletedTasks = append(l.session.CompletedTasks,
			fmt.Sprintf("%s: %s", agent.Config.Role, taskSummary))
	}
	// 최근 20개만 유지
	if len(l.session.CompletedTasks) > 20 {
		l.session.CompletedTasks = l.session.CompletedTasks[len(l.session.CompletedTasks)-20:]
	}

	// 3. 프로젝트 요약 갱신 (Claude에게 요청)
	l.refreshProjectSummary(userInput, report)

	// 4. 이슈 추출 (reviewer/consumer 결과에서)
	l.extractIssues(results)

	l.saveSession()
}

func (l *LeadAgent) refreshProjectSummary(userInput, report string) {
	truncReport := report
	if len(truncReport) > 1500 {
		truncReport = truncReport[:1500]
	}

	currentSummary := l.session.ProjectSummary
	prompt := fmt.Sprintf(`아래 정보를 바탕으로 프로젝트의 현재 상태를 1~2문장으로 요약하세요.
다른 텍스트 없이 요약만 출력하세요.

[기존 프로젝트 상태]
%s

[이번 요청]
%s

[이번 결과 보고서]
%s`, currentSummary, userInput, truncReport)

	result := l.callClaudeTextOnly(prompt, 60)
	if result != "" {
		l.session.ProjectSummary = result
	}
}

func (l *LeadAgent) extractIssues(results map[string]string) {
	// consumer(리뷰어 등) 결과에서 이슈 추출
	for aid, output := range results {
		agent, ok := l.Agents[aid]
		if !ok || !agent.Config.IsConsumer {
			continue
		}
		if len(output) < 20 {
			continue
		}

		truncOutput := output
		if len(truncOutput) > 1500 {
			truncOutput = truncOutput[:1500]
		}

		prompt := fmt.Sprintf(`아래 리뷰 결과에서 발견된 이슈를 JSON 배열로 추출하세요.
이슈가 없으면 빈 배열 []을 반환하세요.
반드시 JSON만 출력하세요.

형식:
[{"id": "issue-NNN", "severity": "high|medium|low", "description": "이슈 설명", "file": "파일경로"}]

[리뷰 결과]
%s`, truncOutput)

		raw := l.callClaudeTextOnly(prompt, 60)
		if raw == "" {
			continue
		}
		jsonStr := extractJSON(raw)

		var issues []struct {
			ID          string `json:"id"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			File        string `json:"file"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &issues); err != nil {
			continue
		}

		for _, issue := range issues {
			l.session.OpenIssues = append(l.session.OpenIssues, OpenIssue{
				ID:          issue.ID,
				FoundBy:     agent.Config.Role,
				Severity:    issue.Severity,
				Description: issue.Description,
				File:        issue.File,
				Status:      "open",
			})
		}
	}
}

// resolveIssues marks issues as resolved when fix tasks are completed.
func (l *LeadAgent) resolveIssues(results map[string]string) {
	// 수정 태스크 완료 후, 관련 이슈를 resolved로 변경
	for _, issue := range l.session.OpenIssues {
		if issue.Status != "open" {
			continue
		}
		for _, output := range results {
			if strings.Contains(output, issue.File) || strings.Contains(output, issue.ID) {
				issue.Status = "resolved"
			}
		}
	}
}

// ── 태스크 분해 ──

func (l *LeadAgent) Decompose(userInput string) []Step {
	agentList, _ := json.MarshalIndent(l.listAgents(), "", "  ")
	contextBlock := l.buildSessionBlock()

	prompt := fmt.Sprintf(`%s

[현재 팀원 목록]
%s
%s
[사용자 요구사항]
%s

먼저 요구사항을 분석하세요:
1. 이것이 개발 태스크인지 판단 (인사, 잡담, 질문이면 빈 배열)
2. 개발 태스크라면 어떤 팀원에게 어떤 작업을 배분할지 설명
3. 작업 간 의존관계를 설명

분석을 마친 후, `+"```json"+` 블록으로 결과를 출력하세요.

개발 태스크가 아니면:
`+"```json"+`
[]
`+"```"+`

개발 태스크이면:
`+"```json"+`
[
  {"id": "t1", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": []},
  {"id": "t2", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": ["t1"]}
]
`+"```"+`

규칙:
- 세션에 미해결 이슈가 있고 "고쳐줘", "이어서" 등이면 개발 태스크.
- 각 instruction은 간결하게 핵심만.`, leadIdea, string(agentList), contextBlock, userInput)

	// thinking + 분석 텍스트 모두 스트리밍 (📊 마커로 thinking 그룹에 포함)
	var raw string
	if l.activeLogFn != nil {
		raw = l.callClaudeStream(prompt, 120, leadToolsReadOnly, func(text string) {
			l.activeLogFn("  📊 " + text)
		})
	} else {
		raw = l.callClaudeBlocking(prompt, leadToolsReadOnly)
	}
	if raw == "" {
		return l.fallbackDecompose(userInput)
	}

	// JSON 블록 추출
	jsonStr := extractJSON(raw)

	var tasks []RawTask
	if err := json.Unmarshal([]byte(jsonStr), &tasks); err != nil {
		// parse failed, using fallback
		return l.fallbackDecompose(userInput)
	}

	if len(tasks) == 0 {
		return []Step{}
	}

	// 유효한 에이전트만
	validIDs := make(map[string]bool)
	for id := range l.Agents {
		validIDs[id] = true
	}
	var filtered []RawTask
	for _, t := range tasks {
		if validIDs[t.AgentID] {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		return []Step{}
	}

	return l.toposortToSteps(filtered)
}

func (l *LeadAgent) toposortToSteps(tasks []RawTask) []Step {
	taskMap := make(map[string]RawTask)
	for _, t := range tasks {
		taskMap[t.ID] = t
	}

	validIDs := make(map[string]bool)
	for _, t := range tasks {
		validIDs[t.ID] = true
	}

	// 진입 차수
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	for id := range validIDs {
		inDegree[id] = 0
		dependents[id] = nil
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if validIDs[dep] {
				inDegree[t.ID]++
				dependents[dep] = append(dependents[dep], t.ID)
			}
		}
	}

	// BFS 웨이브
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var waves [][]RawTask
	visited := make(map[string]bool)

	for len(queue) > 0 {
		waveSize := len(queue)
		var waveTasks []RawTask
		var nextQueue []string

		for i := 0; i < waveSize; i++ {
			tid := queue[i]
			visited[tid] = true
			waveTasks = append(waveTasks, taskMap[tid])
			for _, depTID := range dependents[tid] {
				inDegree[depTID]--
				if inDegree[depTID] == 0 {
					nextQueue = append(nextQueue, depTID)
				}
			}
		}
		waves = append(waves, waveTasks)
		queue = nextQueue
	}

	// 순환 의존성 검출
	if len(visited) < len(validIDs) {
		var orphans []RawTask
		for id := range validIDs {
			if !visited[id] {
				orphans = append(orphans, taskMap[id])
			}
		}
		// cyclic dependency detected, appending orphans to last wave
		waves = append(waves, orphans)
	}

	// Step 포맷으로 변환
	var steps []Step
	for i, wave := range waves {
		var agentNames []string
		var stepTasks []StepTask
		for _, t := range wave {
			agentNames = append(agentNames, t.AgentID)
			stepTasks = append(stepTasks, StepTask{AgentID: t.AgentID, Instruction: t.Instruction})
		}
		steps = append(steps, Step{
			StepNum: i + 1,
			Title:   fmt.Sprintf("Wave %d (%s)", i+1, strings.Join(agentNames, ", ")),
			Tasks:   stepTasks,
		})
	}
	return steps
}

func (l *LeadAgent) fallbackDecompose(userInput string) []Step {
	// fallback: sending same instruction to all agents
	var tasks []StepTask
	for id, agent := range l.Agents {
		tasks = append(tasks, StepTask{
			AgentID:     id,
			Instruction: fmt.Sprintf("다음 요구사항에서 당신의 역할(%s)에 해당하는 부분을 수행하세요:\n\n%s", agent.Config.Role, userInput),
		})
	}
	return []Step{{StepNum: 1, Title: "전체 작업", Tasks: tasks}}
}

// ── 계약서 생성 ──

func (l *LeadAgent) GenerateContract(userInput string, plan []Step) string {
	agentList, _ := json.MarshalIndent(l.listAgents(), "", "  ")
	planJSON, _ := json.MarshalIndent(plan, "", "  ")

	prompt := fmt.Sprintf(`당신은 소프트웨어 아키텍트입니다.
아래 프로젝트 계획을 보고, 모든 팀원이 따라야 할 인터페이스 계약서를 작성하세요.

[사용자 요구사항]
%s

[실행 계획]
%s

[팀원 목록]
%s

먼저 프로젝트 구조를 분석하고, 팀원들 간 공유해야 할 인터페이스를 설명하세요.
그 다음 `+"`"+`yaml 블록으로 계약서를 출력하세요.

계약서에 포함할 내용 (해당하는 것만):
1. tech_stack: 사용할 언어, 프레임워크
2. naming_conventions: 필드명 규칙
3. api_endpoints 또는 shared_interfaces: 주요 인터페이스
4. shared_types: 공유 타입 정의

규칙: 간결하게 핵심만. 50줄 이내.`, userInput, string(planJSON), string(agentList))

	// thinking + 분석 텍스트 모두 스트리밍 (📊 마커로 thinking 그룹에 포함)
	var raw string
	if l.activeLogFn != nil {
		raw = l.callClaudeStream(prompt, 120, leadToolsReadOnly, func(text string) {
			l.activeLogFn("  📊 " + text)
		})
	} else {
		raw = l.callClaudeBlocking(prompt, leadToolsReadOnly)
	}
	if raw == "" {
		return ""
	}

	return extractYAML(raw)
}

// ── 팀장 직접 응답 ──

func (l *LeadAgent) DirectReply(userInput string) string {
	contextBlock := l.buildSessionBlock()
	prompt := fmt.Sprintf(`당신은 소프트웨어 개발 팀의 팀장입니다.
사용자의 메시지에 친절하게 한국어로 응답하세요.
개발 관련 요청이 필요하면 어떤 것을 도와줄 수 있는지 안내해주세요.
%s
[사용자 메시지]
%s`, contextBlock, userInput)

	result := l.callClaudeInteract(prompt, 60)
	if result != "" {
		return result
	}
	return "안녕하세요! 개발 관련 요청을 입력해주시면 팀원들에게 배분하여 처리하겠습니다."
}

// SaveDirectReply saves a non-dev conversation to session.
func (l *LeadAgent) SaveDirectReply(userInput, reply string) {
	l.addConversation("user", userInput)
	l.addConversation("lead", reply)
	// 프로젝트 요약도 갱신 (파일 읽기 등 중요한 컨텍스트일 수 있음)
	if l.session.ProjectSummary == "" && len(reply) > 50 {
		l.refreshProjectSummary(userInput, reply)
	}
	l.saveSession()
}

// ── 단계 실행 ──

func (l *LeadAgent) executeStep(tasks []StepTask, logFn LogFunc) map[string]string {
	results := make(map[string]string)
	channels := make(map[string]chan string)

	for _, task := range tasks {
		agent, ok := l.Agents[task.AgentID]
		if !ok {
			// unknown agent, skipping
			continue
		}
		agent.Reset()
		ch := agent.RunAsync(task.Instruction, logFn)
		channels[task.AgentID] = ch
	}

	for aid, ch := range channels {
		results[aid] = <-ch
	}
	return results
}

// ── 결과 요약 ──

func (l *LeadAgent) summarize(userInput string, results map[string]string, logFn ...LogFunc) string {
	var parts []string
	for aid, output := range results {
		if agent, ok := l.Agents[aid]; ok {
			parts = append(parts, fmt.Sprintf("[%s 결과]\n%s", agent.Config.Role, output))
		}
	}
	resultsText := strings.Join(parts, "\n\n")

	prompt := fmt.Sprintf(`당신은 소프트웨어 개발 팀의 팀장입니다.
팀원들의 작업 결과를 취합하여 사용자에게 전달할 최종 보고서를 작성하세요.

[원래 사용자 요청]
%s

[팀원별 작업 결과]
%s

위 내용을 바탕으로:
1. 완료된 작업 요약
2. 각 팀원이 수행한 내용
3. 전체 결과물에 대한 평가
4. 다음 단계 제안

형식으로 한국어 보고서를 작성하세요.`, userInput, resultsText)

	report := l.callClaudeTextOnly(prompt, 120)

	if report != "" {
		l.updateSession(userInput, results, report)
		return report
	}

	fallback := fmt.Sprintf("[팀원 결과 요약]\n\n%s", resultsText)
	l.updateSession(userInput, results, fallback)
	return fallback
}

// ── Claude CLI 호출 헬퍼 ──
//
// 팀장의 도구 권한은 용도별로 분리됩니다:
//   - 읽기 전용 (계획 수립): Read, Glob, Grep — 프로젝트 구조 탐색만 가능
//   - 대화용 (DirectReply):  Read, Glob, Grep, Bash — 탐색 + 명령 실행
//   - 텍스트 전용 (요약 등): 도구 없음 — 순수 텍스트 생성만
// 모든 호출은 cmd.Dir = WorkDir(프로젝트 디렉토리)로 고정됩니다.

// 팀장 도구 세트
var (
	leadToolsReadOnly = []string{"Read", "Glob", "Grep"}                 // 계획 수립용
	leadToolsInteract = []string{"Read", "Glob", "Grep", "Bash"}         // 대화/탐색용
	leadToolsNone     []string                                            // 텍스트 생성 전용
)

// callClaude: 읽기 전용 도구로 호출 (계획 수립, 계약서 생성, IsDevTask 등)
// thinking만 스트리밍, 텍스트(분석/JSON 등)는 표시하지 않음
func (l *LeadAgent) callClaude(prompt string, timeoutSec int) string {
	return l.callClaudeWith(prompt, timeoutSec, leadToolsReadOnly)
}

// callClaudeInteract: 대화용 도구로 호출 (DirectReply 등)
// thinking + 텍스트 모두 스트리밍 (사용자에게 보여줄 응답)
func (l *LeadAgent) callClaudeInteract(prompt string, timeoutSec int) string {
	if l.activeLogFn != nil {
		return l.callClaudeStream(prompt, timeoutSec, leadToolsInteract, func(text string) {
			l.activeLogFn("  " + text)
		})
	}
	return l.callClaudeBlocking(prompt, leadToolsInteract)
}

// callClaudeTextOnly: 도구 없이 순수 텍스트 생성 (요약, 이슈 추출 등)
// thinking만 스트리밍, 텍스트는 표시하지 않음
func (l *LeadAgent) callClaudeTextOnly(prompt string, timeoutSec int) string {
	return l.callClaudeWith(prompt, timeoutSec, leadToolsNone)
}

// callClaudeWith: 지정된 도구 세트로 Claude CLI 호출 (thinking만 스트리밍)
func (l *LeadAgent) callClaudeWith(prompt string, timeoutSec int, tools []string) string {
	if l.activeLogFn != nil {
		return l.callClaudeStream(prompt, timeoutSec, tools, nil)
	}
	return l.callClaudeBlocking(prompt, tools)
}

// callClaudeBlocking: 블로킹 호출 (폴백 전용)
func (l *LeadAgent) callClaudeBlocking(prompt string, tools []string) string {
	args := []string{"--print", "--dangerously-skip-permissions"}
	if len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	} else if tools != nil {
		// tools가 빈 슬라이스(명시적 "도구 없음") → 아무 도구도 허용하지 않음
		args = append(args, "--allowedTools", "")
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = l.WorkDir
	cmd.Stdin = strings.NewReader(prompt)

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// callClaudeStream: 실시간 스트리밍
// onText가 nil이면 thinking만 스트리밍 (계획 수립 등), non-nil이면 텍스트도 스트리밍 (대화용)
func (l *LeadAgent) callClaudeStream(prompt string, timeoutSec int, tools []string, onText func(string)) string {
	args := []string{"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--dangerously-skip-permissions",
	}
	if len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	} else if tools != nil {
		args = append(args, "--allowedTools", "")
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = l.WorkDir
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return l.callClaudeBlocking(prompt, tools)
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return l.callClaudeBlocking(prompt, tools)
	}

	var fullResult string
	textStarted := false
	thinkStarted := false
	ParseStream(stdout, StreamCallbacks{
		OnText: func(text string) {
			thinkStarted = false
			if onText != nil {
				if !textStarted {
					onText(text)
					textStarted = true
				} else {
					if l.activeLogFn != nil {
						l.activeLogFn("\x01" + text)
					}
				}
			}
		},
		OnThinking: func(text string) {
			textStarted = false
			if l.activeLogFn != nil {
				if !thinkStarted {
					l.activeLogFn("  💭 " + text)
					thinkStarted = true
				} else {
					l.activeLogFn("\x01" + text)
				}
			}
		},
		OnToolUse: func(toolName string, input string) {
			textStarted = false
			thinkStarted = false
			if l.activeLogFn != nil {
				msg := "  🔧 " + toolName
				if input != "" {
					msg += ": " + input
				}
				l.activeLogFn(msg)
			}
		},
		OnResult: func(result string) {
			textStarted = false
			thinkStarted = false
			fullResult = result
		},
	})

	cmd.Wait()
	return strings.TrimSpace(fullResult)
}
