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

// ── LeadAgent ──

type LeadAgent struct {
	WorkDir     string
	Agents      map[string]*Agent
	lastContext string // 대화 메모리
	mu          sync.Mutex
}

func NewLeadAgent(workDir string) *LeadAgent {
	os.MkdirAll(workDir, 0755)
	return &LeadAgent{
		WorkDir: workDir,
		Agents:  make(map[string]*Agent),
	}
}

func (l *LeadAgent) AddAgent(agent *Agent) {
	l.Agents[agent.Config.AgentID] = agent
	fmt.Printf("[팀장] 팀원 추가: %s (id=%s)\n", agent.Config.Role, agent.Config.AgentID)
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

// ── 핵심: 사용자 입력 처리 ──

// LogFunc is called for real-time log streaming to GUI
type LogFunc func(msg string)

func (l *LeadAgent) Process(userInput string, logFn LogFunc) string {
	if logFn == nil {
		logFn = func(msg string) { fmt.Println(msg) }
	}

	logFn(fmt.Sprintf("\n%s", strings.Repeat("=", 60)))
	logFn(fmt.Sprintf("[팀장] 사용자 입력 수신: %s", userInput))
	logFn(strings.Repeat("=", 60))

	// 1. 계획 수립
	logFn("\n[팀장] 📋 실행 계획 수립 중...")
	plan := l.decompose(userInput)
	if plan == nil {
		return "계획 수립에 실패했습니다."
	}
	if len(plan) == 0 {
		logFn("[팀장] 💬 개발 태스크가 아닙니다. 팀장이 직접 응답합니다.")
		return l.directReply(userInput)
	}

	// 2. 계약서 생성
	logFn("\n[팀장] 📜 인터페이스 계약서 작성 중...")
	contract := l.generateContract(userInput, plan)

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

		results := l.executeStep(step.Tasks)
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
		return l.summarize(userInput, allResults)
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
	fmt.Printf("[팀장] 📜 계약서 저장: %s\n", filepath.Join(dir, "contract.yaml"))
}

// ── 대화 메모리 ──

func (l *LeadAgent) saveContext(userInput, report string) {
	truncReport := report
	if len(truncReport) > 2000 {
		truncReport = truncReport[:2000]
	}

	prompt := fmt.Sprintf(`아래 보고서에서 핵심 정보만 추출하여 5줄 이내로 요약하세요.
반드시 포함: 1) 무엇을 만들었는지 2) 해결이 필요한 문제 목록 3) 다음에 해야 할 일
다른 텍스트 없이 요약만 출력하세요.

[사용자 요청]
%s

[보고서]
%s`, userInput, truncReport)

	result := l.callClaude(prompt, 60)
	if result != "" {
		l.lastContext = result
		return
	}

	// 폴백
	fallback := report
	if len(fallback) > 500 {
		fallback = fallback[:500]
	}
	l.lastContext = fmt.Sprintf("[이전 요청: %s]\n%s", userInput, fallback)
}

func (l *LeadAgent) buildContextBlock() string {
	if l.lastContext == "" {
		return ""
	}
	return fmt.Sprintf("\n[이전 작업 컨텍스트 — 사용자가 이전 작업을 참조할 수 있습니다]\n%s\n", l.lastContext)
}

// ── 태스크 분해 ──

func (l *LeadAgent) decompose(userInput string) []Step {
	agentList, _ := json.MarshalIndent(l.listAgents(), "", "  ")
	contextBlock := l.buildContextBlock()

	prompt := fmt.Sprintf(`%s

[현재 팀원 목록]
%s
%s
[사용자 요구사항]
%s

중요 규칙:
- 개발/구현/설계 등 실제 작업이 필요한 요청만 태스크로 분해하세요.
- 인사, 잡담, 단순 질문 등 개발 태스크가 아닌 경우 반드시 빈 배열 []만 반환하세요.
- "안녕", "뭐해?", "고마워" 같은 입력에는 절대 태스크를 생성하지 마세요. []를 반환하세요.
- 단, "이전 작업 컨텍스트"가 있고, 사용자가 "고쳐줘", "수정해", "해결해" 등 이전 작업을 참조하는 경우에는 개발 태스크입니다. 컨텍스트의 문제점을 기반으로 태스크를 분해하세요.

작업 계획 규칙:
- 각 태스크에 고유 id를 부여하세요 (예: "t1", "t2", ...).
- depends_on에 선행 태스크 id를 명시하세요. 의존성이 없으면 빈 배열 [].
- 시스템이 의존성을 분석하여 자동으로 병렬 실행합니다. 단계 번호는 불필요합니다.
- 각 instruction은 간결하게 핵심만. 전체 코드가 아닌 핵심 구조만 지시하세요.

개발 태스크인 경우에만 아래 형식으로 응답하세요:
[
  {"id": "t1", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": []},
  {"id": "t2", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": ["t1"]},
  {"id": "t3", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": ["t1"]},
  {"id": "t4", "agent_id": "팀원ID", "instruction": "구체적인 지시", "depends_on": ["t2", "t3"]}
]`, leadIdea, string(agentList), contextBlock, userInput)

	raw := l.callClaude(prompt, 120)
	if raw == "" {
		fmt.Println("[팀장] ❌ 태스크 분해 오류")
		return l.fallbackDecompose(userInput)
	}

	// JSON 블록 추출
	re := regexp.MustCompile("(?s)```json\\s*|\\s*```")
	raw = strings.TrimSpace(re.ReplaceAllString(raw, ""))

	var tasks []RawTask
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		fmt.Printf("[팀장] ⚠️  파싱 실패 (%s), 폴백 모드 사용\n", err)
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
		fmt.Printf("[팀장] ⚠️  순환 의존성 감지, 남은 태스크를 마지막 웨이브에 추가\n")
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
	fmt.Println("[팀장] ⚠️  폴백: 모든 팀원에게 동일 지시 전달")
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

func (l *LeadAgent) generateContract(userInput string, plan []Step) string {
	agentList, _ := json.MarshalIndent(l.listAgents(), "", "  ")
	planJSON, _ := json.MarshalIndent(plan, "", "  ")

	prompt := fmt.Sprintf(`당신은 소프트웨어 아키텍트입니다.
아래 프로젝트 계획을 보고, 모든 팀원이 따라야 할 인터페이스 계약서를 YAML 형식으로 작성하세요.

[사용자 요구사항]
%s

[실행 계획]
%s

[팀원 목록]
%s

계약서에 반드시 포함할 내용:
1. tech_stack: 사용할 언어, 프레임워크, DB 엔진
2. naming_conventions: 필드명 규칙 (camelCase/snake_case), 언어 (한글/영문)
3. api_endpoints: 주요 엔드포인트 목록 (method, path, request/response 필드명)
4. db_schema: 주요 테이블/컬렉션과 필드명
5. shared_types: 공유 타입 정의 (예: User, Course 등)

규칙:
- 간결하게 핵심만 작성하세요. 50줄 이내로.
- YAML만 출력하세요. 마크다운 펜스나 설명 텍스트는 포함하지 마세요.`, userInput, string(planJSON), string(agentList))

	raw := l.callClaude(prompt, 120)
	if raw == "" {
		fmt.Println("[팀장] ⚠️  계약서 생성 실패, 계약서 없이 진행합니다.")
		return ""
	}

	re := regexp.MustCompile("(?s)```ya?ml\\s*|\\s*```")
	return strings.TrimSpace(re.ReplaceAllString(raw, ""))
}

// ── 팀장 직접 응답 ──

func (l *LeadAgent) directReply(userInput string) string {
	contextBlock := l.buildContextBlock()
	prompt := fmt.Sprintf(`당신은 소프트웨어 개발 팀의 팀장입니다.
사용자의 메시지에 친절하게 한국어로 응답하세요.
개발 관련 요청이 필요하면 어떤 것을 도와줄 수 있는지 안내해주세요.
%s
[사용자 메시지]
%s`, contextBlock, userInput)

	result := l.callClaude(prompt, 60)
	if result != "" {
		return result
	}
	return "안녕하세요! 개발 관련 요청을 입력해주시면 팀원들에게 배분하여 처리하겠습니다."
}

// ── 단계 실행 ──

func (l *LeadAgent) executeStep(tasks []StepTask) map[string]string {
	results := make(map[string]string)
	channels := make(map[string]chan string)

	for _, task := range tasks {
		agent, ok := l.Agents[task.AgentID]
		if !ok {
			fmt.Printf("  ⚠️  알 수 없는 에이전트: %s\n", task.AgentID)
			continue
		}
		agent.Reset()
		ch := agent.RunAsync(task.Instruction)
		channels[task.AgentID] = ch
	}

	for aid, ch := range channels {
		results[aid] = <-ch
	}
	return results
}

// ── 결과 요약 ──

func (l *LeadAgent) summarize(userInput string, results map[string]string) string {
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

	report := l.callClaude(prompt, 120)
	if report != "" {
		l.saveContext(userInput, report)
		return report
	}

	fallback := fmt.Sprintf("[팀원 결과 요약]\n\n%s", resultsText)
	l.saveContext(userInput, fallback)
	return fallback
}

// ── Claude CLI 호출 헬퍼 ──

func (l *LeadAgent) callClaude(prompt string, timeoutSec int) string {
	cmd := exec.Command("claude", "--print", "--dangerously-skip-permissions")
	cmd.Dir = l.WorkDir
	cmd.Stdin = strings.NewReader(prompt)

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
