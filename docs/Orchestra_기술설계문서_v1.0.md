# ORCHESTRA
## Claude Code Multi-Agent Orchestration System
### 기술 설계 문서 v1.0

| 항목 | 내용 |
|------|------|
| 프로젝트명 | Orchestra |
| 언어 | Go + React (Wails v2) |
| 대상 플랫폼 | Linux / Windows |
| AI 엔진 | Claude Code CLI |
| 문서 버전 | 1.0.0 |

---

# 1. 프로젝트 개요 (Project Overview)

## 1.1 목표 (Objective)

Orchestra는 여러 Claude Code 인스턴스를 오케스트레이션하여 복잡한 소프트웨어 개발 작업을 자동화하는 GUI 기반 멀티 에이전트 시스템입니다.

사용자는 팀장(Lead Agent)과 다양한 역할의 팀원(Sub-Agent) 노드를 구성하고, 팀장 AI가 사용자의 요구사항을 분석하여 각 팀원에게 최적화된 태스크를 배분합니다.

> 💡 **핵심 컨셉**: 팀장 AI = 오케스트레이터 / 팀원 AI = 전문 실행자. 각자 격리된 워크스페이스에서 작업하며 팀장이 전체를 조율합니다.

## 1.2 주요 특징

- Claude Code CLI를 subprocess로 래핑하여 각 에이전트 독립 실행
- React Flow 기반 노드 에디터로 팀 구성 시각화
- 역할별 이데아(Idea) 시스템 — 사용자 편집 가능한 시스템 프롬프트
- 디렉토리 격리로 팀원 간 워크스페이스 충돌 방지
- fsnotify 기반 파일 변경 감지 및 작업 완료 신호 처리
- 코드 리뷰어, 문서 작성자 등 크로스 참조 에이전트 지원

## 1.3 기술 스택 요약

| 레이어 | 기술 | 선택 이유 |
|--------|------|-----------|
| GUI 프레임워크 | Wails v2 | Go 네이티브 + React, 크로스플랫폼 |
| 노드 에디터 | React Flow | 에이전트 노드 시각화 및 연결 |
| AI 엔진 | Claude Code CLI | subprocess + PTY로 래핑 |
| 상태 관리 | Go channels / goroutines | 비동기 에이전트 응답 처리 |
| 파일 감시 | fsnotify | 작업 완료 감지, 충돌 방지 |
| 이데아 저장 | JSON / YAML | 역할별 시스템 프롬프트 관리 |

---

# 2. 시스템 아키텍처

## 2.1 전체 구조도

```
┌─────────────────────────────────────────────────────┐
│                   GUI Layer (Wails)                  │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │  Node Editor  │  │ Idea Editor  │  │  Log View │  │
│  │ (React Flow)  │  │  (textarea)  │  │ (stream)  │  │
│  └──────┬────────┘  └──────┬───────┘  └─────┬─────┘  │
└─────────┼──────────────────┼────────────────┼────────┘
          │                  │                │
┌─────────▼──────────────────▼────────────────▼────────┐
│            Orchestration Engine (Go)                  │
│  ┌─────────────────────────────────────────────────┐  │
│  │              AgentManager                        │  │
│  │  ┌────────────┐    ┌────────────────────────┐   │  │
│  │  │ LeadAgent  │───▶│  TaskRouter / Scheduler │   │  │
│  │  └────────────┘    └──────────┬─────────────┘   │  │
│  │                               │                  │  │
│  │  ┌────────────────────────────▼───────────────┐  │  │
│  │  │           SubAgent Pool                     │  │  │
│  │  │  [Backend] [Frontend] [DB] [Doc] [Reviewer] │  │  │
│  │  └────────────────────────────────────────────┘  │  │
│  └─────────────────────────────────────────────────┘  │
└───────────────────────────┬───────────────────────────┘
                            │
┌───────────────────────────▼───────────────────────────┐
│           Agent Execution Layer                        │
│  Each agent: claude-code CLI subprocess + PTY          │
│  Isolated working directory per agent                  │
└───────────────────────────────────────────────────────┘
```

## 2.2 워크스페이스 디렉토리 구조

팀장은 메인 디렉토리 전체를 playground로 사용하며, 각 팀원은 자신의 서브 디렉토리만 접근합니다. 단, 코드 리뷰어와 문서 작성자는 다른 서브 디렉토리를 읽기 전용으로 참조합니다.

```
MainWorkspace/                    ← 팀장(Lead) playground
│
├── .orchestra/                   ← Orchestra 내부 설정
│   ├── config.yaml               ← 프로젝트 설정
│   ├── ideas/                    ← 에이전트별 이데아(시스템 프롬프트)
│   │   ├── lead.yaml
│   │   ├── backend.yaml
│   │   └── frontend.yaml
│   ├── tasks/                    ← 태스크 그래프 & 상태
│   └── locks/                    ← 파일 락 레지스트리
│
├── backend-expert/               ← Backend 팀원 playground
│   ├── src/
│   └── .agent-status             ← 작업 완료 신호 파일
│
├── frontend-expert/              ← Frontend 팀원 playground
│   ├── components/
│   └── .agent-status
│
├── db-master/                    ← DB 팀원 playground
│   ├── migrations/
│   └── .agent-status
│
├── doc-writer/                   ← 문서 작성자 (읽기 전용 크로스 참조)
│   ├── docs/
│   └── .agent-status             ← backend/, frontend/ 읽기 참조
│
└── code-reviewer/                ← 코드 리뷰어 (읽기 전용 크로스 참조)
    ├── reviews/
    └── .agent-status             ← 모든 서브 dirs 읽기 참조
```

## 2.3 에이전트 유형 분류

| 유형 | 예시 역할 | 워크스페이스 권한 | 크로스 참조 |
|------|-----------|------------------|------------|
| Producer | Backend, Frontend, DB Master | 자신의 서브 dir 읽기/쓰기 | 없음 |
| Consumer | Code Reviewer, Doc Writer | 자신의 서브 dir 읽기/쓰기 | 다른 서브 dirs 읽기 전용 |
| Lead | 팀장 (Orchestrator) | 메인 dir 전체 읽기/쓰기 | 모든 dirs |

---

# 3. 핵심 컴포넌트 설계

## 3.1 AgentNode 구조체 (Go)

각 에이전트는 `AgentNode` 구조체로 표현됩니다. Claude Code CLI 프로세스, I/O 파이프, 이데아, 상태를 포함합니다.

```go
// AgentNode represents a single Claude Code agent instance
type AgentNode struct {
    ID          string           // 고유 식별자
    Role        string           // 역할명 (e.g., "backend-expert")
    RoleType    AgentRoleType    // Producer | Consumer | Lead
    WorkDir     string           // 격리된 작업 디렉토리 경로
    Idea        string           // 사용자 편집 가능한 시스템 프롬프트

    // Process management
    Process     *os.Process
    PTY         *os.File         // Pseudo-terminal
    Stdin       io.WriteCloser
    Stdout      io.ReadCloser

    // State
    Status      AgentStatus      // Idle | Running | Waiting | Done | Error
    CurrentTask *Task
    OutputChan  chan string       // 실시간 출력 스트림
    DoneChan    chan struct{}     // 작업 완료 신호

    // Cross-reference (Consumer 타입만)
    ReadRefs    []string         // 읽기 전용 참조 디렉토리 목록
    mu          sync.RWMutex
}
```

## 3.2 이데아(Idea) 시스템

이데아는 각 에이전트의 역할, 전문성, 행동 방침을 정의하는 시스템 프롬프트입니다. 팀장 AI가 역할에 맞는 기본 이데아를 자동 생성하며, 사용자가 자유롭게 편집할 수 있습니다.

> ✏️ **편집**: GUI의 Idea Editor에서 각 노드를 클릭하면 해당 에이전트의 이데아를 실시간으로 편집할 수 있습니다.

```yaml
# .orchestra/ideas/backend-expert.yaml
role: backend-expert
version: 1
idea: |
  당신은 백엔드 개발 전문가입니다.
  담당 디렉토리: ./backend-expert/

  전문 영역:
  - RESTful API 설계 및 구현 (Go, Node.js, Python)
  - 인증/인가 시스템 (JWT, OAuth2)
  - 성능 최적화 및 캐싱 전략

  작업 규칙:
  - 자신의 디렉토리(./backend-expert/) 외부 파일은 수정하지 않습니다.
  - 작업 완료 시 .agent-status 파일에 DONE을 기록합니다.
  - 다른 팀원과의 인터페이스는 .orchestra/contracts/ 에 명세합니다.
```

## 3.3 팀장 AI의 태스크 분배 로직

팀장 AI는 사용자 입력을 받아 다음 파이프라인을 통해 각 팀원에게 태스크를 배분합니다.

```
User Input
    │
    ▼
┌─────────────────────────────────────┐
│  Lead Agent (Claude Code)            │
│  1. 요구사항 분석                     │
│  2. 태스크 분해 (Task Decomposition)  │
│  3. 의존성 그래프 생성                │
│  4. 팀원 역할 매핑                   │
│  5. 실행 순서 결정 (순차/병렬)        │
└──────────────┬──────────────────────┘
               │ TaskGraph
               ▼
    ┌──────────────────────┐
    │    TaskScheduler      │
    │  의존성 해결 후 실행   │
    └──────────┬───────────┘
               │
    ┌──────────┴──────────┐
    ▼          ▼          ▼
 [Backend]  [Frontend]  [DB]    ← 병렬 실행 가능
    │
    ▼ 완료 후
 [Reviewer] [DocWriter]         ← 순차 실행 (의존성)
```

## 3.4 태스크 그래프 (Task Graph)

```go
type Task struct {
    ID           string
    AgentID      string
    Instruction  string       // 팀원에게 전달할 정제된 지시
    Dependencies []string     // 선행 태스크 ID 목록
    Status       TaskStatus
    Priority     int
}

type TaskGraph struct {
    Tasks    map[string]*Task
    Edges    map[string][]string  // task → depends on tasks
    mu       sync.RWMutex
}

// 위상 정렬로 실행 순서 결정
func (g *TaskGraph) TopologicalSort() ([][]string, error) {
    // 병렬 실행 가능한 태스크들을 배치(batch)로 반환
    // batch[0] → 병렬 실행 → batch[1] → 병렬 실행 → ...
}
```

---

# 4. Claude Code 프로세스 관리

## 4.1 subprocess + PTY 래핑

Claude Code는 대화형 CLI 도구이므로 일반 `os/exec`만으로는 입출력 처리가 불안정합니다. PTY(Pseudo-Terminal)를 사용하여 안정적인 I/O를 보장합니다.

```go
import (
    "github.com/creack/pty"
    "os/exec"
)

func (a *AgentNode) Start() error {
    cmd := exec.Command("claude",
        "--output-format", "stream-json",
        "--print",
    )
    cmd.Dir = a.WorkDir

    // 환경변수로 이데아 주입
    cmd.Env = append(os.Environ(),
        "ORCHESTRA_IDEA="+a.Idea,
        "ORCHESTRA_AGENT_ID="+a.ID,
    )

    // PTY 생성
    ptmx, err := pty.Start(cmd)
    if err != nil {
        return fmt.Errorf("PTY start failed: %w", err)
    }
    a.PTY = ptmx
    a.Process = cmd.Process

    // 출력 스트리밍 goroutine
    go a.streamOutput(ptmx)
    return nil
}
```

## 4.2 출력 스트리밍 처리

```go
func (a *AgentNode) streamOutput(r io.Reader) {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := scanner.Text()

        // JSON 파싱 시도 (--output-format stream-json)
        var event ClaudeEvent
        if json.Unmarshal([]byte(line), &event) == nil {
            switch event.Type {
            case "text":          // 텍스트 출력
                a.OutputChan <- event.Content
            case "tool_use":      // 도구 사용 감지
                a.handleToolUse(event)
            case "end_turn":      // 작업 완료
                a.DoneChan <- struct{}{}
            }
        } else {
            // 파싱 실패 시 raw 출력 전달
            a.OutputChan <- line
        }
    }
}
```

## 4.3 에이전트에 지시 전달

```go
func (a *AgentNode) SendInstruction(instruction string) error {
    // 이데아 + 지시를 결합하여 전달
    prompt := fmt.Sprintf(`
[SYSTEM IDEA]
%s

[TASK]
%s
`, a.Idea, instruction)

    _, err := fmt.Fprintln(a.PTY, prompt)
    return err
}
```

---

# 5. 워크스페이스 충돌 방지

## 5.1 문제 정의

여러 에이전트가 동시에 같은 파일을 수정하면 데이터 손실 및 충돌이 발생합니다. Orchestra는 세 가지 메커니즘으로 이를 방지합니다.

## 5.2 디렉토리 격리 (1차 방어)

- 각 팀원은 자신의 서브 디렉토리만 작업 디렉토리로 설정
- Claude Code 실행 시 WorkDir을 각자의 서브 디렉토리로 지정
- 이데아에 '자신의 디렉토리 외부 수정 금지' 규칙 명시

## 5.3 파일 락 레지스트리 (2차 방어)

```go
// .orchestra/locks/ 에 락 파일 생성
type FileLockRegistry struct {
    locks map[string]string  // filePath → agentID
    mu    sync.RWMutex
}

func (r *FileLockRegistry) Acquire(filePath, agentID string) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if holder, exists := r.locks[filePath]; exists {
        return fmt.Errorf("file locked by %s", holder)
    }
    r.locks[filePath] = agentID
    return nil
}
```

## 5.4 fsnotify 기반 완료 감지 (3차 방어)

팀장은 파일 시스템 감시를 통해 팀원의 작업 완료를 감지하고, 의존성이 있는 다음 팀원의 실행을 조율합니다.

```go
func (m *AgentManager) WatchAgentStatus(agent *AgentNode) {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(filepath.Join(agent.WorkDir, ".agent-status"))

    go func() {
        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write != 0 {
                    status := readStatusFile(event.Name)
                    if status == "DONE" {
                        agent.DoneChan <- struct{}{}
                        m.scheduler.OnAgentDone(agent.ID)
                    }
                }
            }
        }
    }()
}
```

## 5.5 크로스 참조 에이전트 처리

> 📖 **읽기 전용**: 코드 리뷰어, 문서 작성자는 다른 팀원의 디렉토리를 읽기 전용으로 참조합니다. 팀장이 참조할 파일 경로를 이데아에 동적으로 주입합니다.

```go
// 팀장이 Consumer 에이전트 시작 시 참조 경로 주입
func (m *AgentManager) StartConsumer(agent *AgentNode) {
    refs := m.getReadableRefs(agent)  // 완료된 Producer들의 디렉토리

    enrichedIdea := agent.Idea + "\n\n" +
        "[참조 가능한 읽기 전용 디렉토리]\n" +
        strings.Join(refs, "\n")

    agent.Idea = enrichedIdea
    agent.Start()
}
```

---

# 6. GUI 설계 (Wails v2 + React Flow)

## 6.1 주요 화면 구성

| 화면 | 설명 | 주요 기능 |
|------|------|-----------|
| Node Editor | 에이전트 노드 시각화 | 노드 추가/삭제, 연결선, 드래그 |
| Idea Editor | 이데아 편집 패널 | 노드 클릭 시 열리는 텍스트 편집기 |
| Task Monitor | 태스크 실행 현황 | 태스크 그래프, 진행률, 상태 |
| Log Viewer | 실시간 에이전트 출력 | 에이전트별 로그 스트리밍 |
| Workspace Explorer | 디렉토리 구조 탐색 | 파일 트리, 읽기 전용 구분 표시 |

## 6.2 노드 에디터 컴포넌트 (React Flow)

```jsx
const AgentNode = ({ data }) => {
  const statusColor = {
    idle: '#gray', running: '#blue',
    done: '#green', error: '#red'
  }[data.status];

  return (
    <div className={`agent-node ${data.roleType}`}>
      <div className='node-header' style={{ background: statusColor }}>
        <span>{data.role}</span>
        <StatusBadge status={data.status} />
      </div>
      <div className='node-body'>
        <p>{data.currentTask}</p>
        <button onClick={() => openIdeaEditor(data.id)}>
          이데아 편집
        </button>
      </div>
    </div>
  );
};
```

## 6.3 Wails 백엔드 바인딩

```go
// Go 함수를 React에서 직접 호출
type App struct {
    manager *AgentManager
}

// 노드 추가 — React에서 window.go.App.AddAgent() 로 호출
func (a *App) AddAgent(role string, roleType string) (string, error) {
    return a.manager.CreateAgent(role, roleType)
}

// 이데아 업데이트
func (a *App) UpdateIdea(agentID string, idea string) error {
    return a.manager.UpdateAgentIdea(agentID, idea)
}

// 사용자 입력 처리
func (a *App) SubmitUserInput(input string) error {
    return a.manager.LeadAgent.ProcessUserInput(input)
}

// 실시간 로그 스트리밍 (Wails Event)
func (a *App) streamLogs(agentID string, ch chan string) {
    for log := range ch {
        runtime.EventsEmit(a.ctx, "log:"+agentID, log)
    }
}
```

---

# 7. 개발 단계 로드맵

## Phase 1 — Core Engine (2~3주)

> 🎯 **목표**: Claude Code subprocess 관리 + 디렉토리 격리 + 기본 팀장↔팀원 통신

1. Go 프로젝트 초기화 (Wails v2 scaffolding)
2. AgentNode 구조체 + PTY subprocess 래핑
3. 이데아 YAML 저장/로드 시스템
4. 디렉토리 격리 + 파일 락 레지스트리
5. 팀장 → 팀원 지시 전달 기본 파이프

## Phase 2 — GUI (2~3주)

> 🎯 **목표**: React Flow 노드 에디터 + 이데아 편집 UI + 실시간 로그 뷰어

1. React Flow 노드 에디터 구현
2. 이데아 편집 사이드패널
3. 실시간 로그 스트리밍 (Wails EventsEmit)
4. 워크스페이스 파일 탐색기
5. 태스크 현황 모니터 기본

## Phase 3 — 오케스트레이션 로직 (3~4주)

> 🎯 **목표**: 팀장 AI 태스크 분배 알고리즘 + 의존성 그래프 + 충돌 방지 완성

1. 태스크 그래프 + 위상 정렬 스케줄러
2. 팀장 AI 프롬프트 엔지니어링 (태스크 분해)
3. fsnotify 기반 완료 감지 시스템
4. Consumer 에이전트 크로스 참조 자동 주입
5. 병렬 실행 조율 + 오류 복구

## Phase 4 — 고도화 (지속)

> 🎯 **목표**: 머징 자동화 + 결과물 통합 + UX 완성도

1. 팀원 결과물 자동 머징 (팀장 AI 위임)
2. 작업 이력 및 세션 저장/복원
3. 에이전트 템플릿 라이브러리
4. 플러그인 방식 역할 확장

---

# 8. 주요 도전 과제 & 해결 전략

| 도전 과제 | 난이도 | 해결 전략 |
|-----------|--------|-----------|
| Claude Code stdout 파싱 | 높음 | `--output-format stream-json` 옵션 활용, PTY로 대화형 처리 |
| 작업 완료 감지 신뢰성 | 중간 | `.agent-status` 파일 + fsnotify 조합, timeout 폴백 |
| 팀원 간 파일 충돌 | 중간 | 디렉토리 격리 1차, 파일 락 레지스트리 2차, 이데아 규칙 3차 |
| 팀장 AI 태스크 분해 품질 | 높음 | 구조화된 프롬프트 + JSON 출력 강제 + 검증 로직 |
| Windows PTY 지원 | 중간 | `github.com/creack/pty` (Windows 지원), ConPTY API 폴백 |
| 에이전트 간 인터페이스 계약 | 중간 | `.orchestra/contracts/` 에 API 명세 공유, 팀장이 조율 |

---

# 9. 기술 의존성 목록

| 패키지 | 용도 | 라이선스 |
|--------|------|----------|
| `github.com/wailsapp/wails/v2` | GUI 프레임워크 (Go + React) | MIT |
| `github.com/creack/pty` | Pseudo-terminal (PTY) 지원 | MIT |
| `github.com/fsnotify/fsnotify` | 파일 시스템 감시 | BSD-3 |
| `github.com/google/uuid` | 에이전트 고유 ID 생성 | BSD-3 |
| `gopkg.in/yaml.v3` | 이데아 YAML 저장 | Apache-2.0 |
| `reactflow` | 노드 에디터 UI | MIT |
| `zustand` | React 상태 관리 | MIT |

---

# 10. 미결 사항 & 추후 논의

- Claude Code CLI의 `--output-format stream-json` 옵션 정식 지원 여부 확인 필요
- 팀장 AI의 태스크 분해 프롬프트 최적화 — 실험적 튜닝 필요
- 에이전트 간 계약(interface contract) 포맷 확정 (JSON Schema vs OpenAPI)
- 멀티 세션 저장/복원 전략 — 프로젝트 단위 vs 세션 단위
- 에이전트 비용 추적 — Claude Code API 사용량 모니터링
- Windows ConPTY 지원 범위 사전 검증
