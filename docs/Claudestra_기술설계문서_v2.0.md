# CLAUDESTRA
## Claude Code Multi-Agent Orchestration System
### 기술 설계 문서 v2.0

| 항목 | 내용 |
|------|------|
| 프로젝트명 | Claudestra |
| MVP 언어 | Python |
| 최종 언어 | Go + React (Wails v2) |
| 대상 플랫폼 | Linux / Windows |
| AI 엔진 | Claude Code CLI |
| 에이전트 격리 | subprocess (현재) → Docker 컨테이너 (최종) |
| 팀장 도구 방식 | Claudestra CLI 바이너리 (v2.0 핵심) |
| 문서 버전 | 2.1.0 |

---

## v2.0 변경 사항 요약

### 핵심: 팀장 CLI 도구화 — 토큰 사용량 대폭 절감

v1.4까지의 팀장은 Go 코드가 Claude CLI를 7~8회 개별 호출하는 구조였습니다.
매 호출마다 전체 컨텍스트(프로젝트 상태, 팀원 목록, 사용자 입력)를 중복 주입해야 했고, 이는 토큰 폭발의 원인이었습니다.

v2.0에서는 팀장 Claude가 **단일 세션** 안에서 `claudestra` CLI를 Bash 도구로 직접 호출합니다.
Claude가 필요한 시점에 필요한 명령만 실행하므로 컨텍스트 중복이 제거됩니다.

**v2.1에서 Phase A~E 전체 구현이 완료되었습니다.**

### 부가 변경: 독립 토글, 분석 텍스트 그룹화

- 각 thinking 그룹이 독립적으로 접기/펼치기 가능 (기존: 전체 동시 토글)
- Decompose/GenerateContract의 분석 텍스트가 📊 마커로 thinking 그룹에 포함

---

# 1. 프로젝트 개요

## 1.1 목표

Claudestra는 여러 Claude Code 인스턴스를 오케스트레이션하여 복잡한 소프트웨어 개발 작업을 자동화하는 멀티 에이전트 시스템입니다.

사용자는 팀장(Lead Agent)과 다양한 역할의 팀원(Sub-Agent) 노드를 구성하고,
팀장 AI가 사용자의 요구사항을 분석하여 각 팀원에게 최적화된 태스크를 배분합니다.

> **핵심 컨셉**: 팀장 AI = 오케스트레이터 / 팀원 AI = 전문 실행자.
> 각자 격리된 워크스페이스에서 작업하며 팀장이 Claudestra CLI 도구를 통해 전체를 조율합니다.

## 1.2 현재 구현 상태 (v1.4 기준)

| 단계 | 상태 |
|------|------|
| Phase 1 — Python MVP | ✅ 완료 |
| Phase 2 — 권한 제어 (`--allowedTools`) | ✅ 완료 |
| Phase 3 — Go + Wails GUI | ✅ 완료 (스트리밍, thinking, 동적 팀 구성, Plan→Approve→Execute) |
| **Phase 3.5 — CLI 도구화** | **✅ 완료 (v2.1)** |
| Phase 4 — Docker 격리 | 미착수 |

---

# 2. CLI 도구화 — v2.0 핵심 설계

## 2.1 기존 구조의 문제

현재 팀장(lead.go)의 Claude 호출 지점:

```
Go 코드 → callClaude (IsDevTask)          ← 항상 실행
Go 코드 → callClaude (PlanTeam)           ← 항상 실행
Go 코드 → callClaudeStream (Decompose)    ← 항상 실행
Go 코드 → callClaudeStream (Contract)     ← 항상 실행
Go 코드 → callClaudeTextOnly (Summary)    ← 항상 실행
Go 코드 → callClaudeTextOnly (RefreshProjectSummary)  ← 항상 실행
Go 코드 → callClaudeTextOnly (ExtractIssues)           ← consumer 있을 때
Go 코드 → callClaudeInteract (DirectReply)             ← 비개발 태스크
```

**문제:**
- 총 7~8회 호출, 매번 풀 컨텍스트(팀원 목록, 세션 상태, 사용자 입력) 중복 주입
- Go 코드가 호출 순서를 하드코딩 → Claude의 판단 여지 없음
- 단순 인사에도 IsDevTask 호출 후 DirectReply 호출 = 최소 2회

## 2.2 새 구조 — Claude가 CLI 도구를 직접 호출

```
새 방식 — 팀장 Claude 단일 세션, 필요할 때만 CLI 호출
──────────────────────────────────────────────────────
Claude → $ claudestra plan "..."         ← 필요할 때만
Claude → $ claudestra assign backend     ← 필요할 때만
Claude → $ claudestra assign frontend    ← 필요할 때만
Claude → $ claudestra status             ← 필요할 때만
Claude → $ claudestra issues             ← 리뷰어 있을 때만
... 필요한 것만 호출, 컨텍스트 1회 주입
```

팀장 Claude는 `--allowedTools Bash`로 실행되며, Bash 도구를 통해 `claudestra` CLI 바이너리를 호출합니다.

## 2.3 아키텍처 변경

```
┌─────────────────────────────────────────────────────┐
│                   GUI Layer (Wails v2)               │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │  Sidebar      │  │ ProposalPanel│  │  LogPanel │  │
│  │  (agents)     │  │  (plan)      │  │ (stream)  │  │
│  └──────┬────────┘  └──────┬───────┘  └─────┬─────┘  │
└─────────┼──────────────────┼────────────────┼────────┘
          │                  │                │
┌─────────▼──────────────────▼────────────────▼────────┐
│            Lead Agent (Claude Code 단일 세션)          │
│                                                       │
│  사용자 입력 수신                                       │
│       │                                               │
│       ▼  Bash 도구로 Claudestra CLI 직접 호출           │
│  $ claudestra plan "..."                              │
│  $ claudestra assign backend "..."                    │
│  $ claudestra status                                  │
│  $ claudestra issues                                  │
│  $ claudestra session get                             │
└───────────────────────────┬───────────────────────────┘
                            │
┌───────────────────────────▼───────────────────────────┐
│          Claudestra CLI (Go 바이너리)                   │
│                                                       │
│  plan / assign / status / issues / session / contract │
└───────────────────────────┬───────────────────────────┘
                            │
         ┌──────────────────┴──────────────────┐
         │                                     │
┌────────▼────────┐                  ┌─────────▼───────┐
│  현재            │                  │  Phase 4         │
│  subprocess     │                  │  Docker          │
│  (claude CLI)   │                  │  컨테이너         │
└─────────────────┘                  └─────────────────┘
```

## 2.4 MCP vs CLI 선택 근거

| | MCP 서버 | Claudestra CLI |
|--|----------|----------------|
| 구현 복잡도 | 높음 (서버 프로세스 관리) | 낮음 (바이너리 하나) |
| 디버깅 | 프로토콜 로그 분석 필요 | 터미널에서 직접 실행 |
| Claude 친화성 | 별도 프로토콜 학습 필요 | CLI 수백만 건 학습됨 |
| 조합성 | 서버 형식에 종속 | pipe, grep, jq 자유롭게 |
| 인증/안정성 | 초기화 불안정, 재인증 빈번 | 바이너리 실행, 상태 없음 |
| 인간 디버깅 | LLM 대화 내부에만 존재 | 사람도 동일 명령어 실행 가능 |

## 2.5 CLI 명령어 (구현 완료)

```bash
claudestra status                         # 팀원 전체 상태 출력
claudestra team                           # 팀원 목록 (JSON)
claudestra session get                    # 세션 메모리 읽기
claudestra session update '<json>'        # 세션 메모리 갱신
claudestra issues                         # 미해결 이슈 목록 출력
claudestra contract get                   # 인터페이스 계약서 조회
claudestra contract set '<yaml>'          # 인터페이스 계약서 설정
claudestra idea    <agent>                # 에이전트 이데아 출력
claudestra output  <agent>                # 에이전트 최근 출력 조회
claudestra assign  <agent> '<instr>'      # 동기 작업 지시 (완료까지 대기)
claudestra assign --async <agent> '<instr>'  # 비동기 작업 지시 (job-id 반환)
claudestra lead-session '<request>'       # 단일 세션 모드 (팀장 자율 실행)
```

모든 출력은 **plain text 또는 JSON** — Claude가 파이프로 조합 가능:

```bash
# status 출력 예시
$ claudestra status
vision_visualization_dev    DONE     ✅
build_qa                    IDLE     ⚪

# 동기 assign — 완료까지 스트리밍 출력 후 결과 반환
$ claudestra assign developer "hello.py 작성"
[developer] 🚀 시작: hello.py 작성...
[developer] 💭 The user wants...
[developer] 🔧 Write: hello.py
--- RESULT ---
hello.py를 작성했습니다.

# 비동기 assign — job-id 즉시 반환, 백그라운드 실행
$ claudestra assign --async developer "hello.py 작성"
a1b2c3d4e5f6
$ cat .orchestra/jobs/job-a1b2c3d4e5f6.json
{"id":"a1b2c3d4e5f6","agent":"developer","status":"done",...}
```

## 2.6 팀장 세션 프롬프트 (`buildLeadSessionPrompt`)

`RunLeadSession` 호출 시 팀장 Claude에게 주입되는 프롬프트. `{CLI}` 플레이스홀더가 실제 바이너리 경로로 치환됩니다:

```
당신은 소프트웨어 개발 팀의 팀장 AI입니다.
Bash 도구를 통해 claudestra CLI로 팀을 관리하고 작업을 수행합니다.

[사용 가능한 CLI 명령어]
{CLI} team / status / session get|update / issues
{CLI} contract get|set / idea <agent> / output <agent>
{CLI} assign <agent> '<instruction>'         (동기)
{CLI} assign --async <agent> '<instruction>' (비동기)

[작업 흐름]
1. {CLI} team 으로 팀 구성 확인
2. {CLI} session get 으로 프로젝트 컨텍스트 파악
3. 사용자 요청 분석
4. 비개발 태스크 → 직접 응답
5. 개발 태스크:
   a. {CLI} idea <agent> 로 역할 확인
   b. {CLI} contract set 으로 계약서 설정
   c. {CLI} assign 으로 작업 지시
   d. 결과 취합 → 보고서 → {CLI} session update
```

팀장 Claude의 `--allowedTools`는 `Bash,Read,Glob,Grep`으로 설정되어 프로젝트 구조 탐색도 가능합니다.

## 2.7 코드 변경 범위 (구현 완료)

| 파일 | 변경 내용 |
|------|-----------|
| `gui/cmd/claudestra/main.go` | **신규** — CLI 바이너리 진입점 (12개 서브커맨드) |
| `gui/internal/jobs.go` | **신규** — Job CRUD (비동기 assign용) |
| `gui/internal/logwatcher.go` | **신규** — fsnotify 기반 JSONL 파일 감시 |
| `gui/internal/lead.go` | `CLIPath` 필드, `RunLeadSession()`, `buildLeadSessionPrompt()` 추가. `resolveIssues` 버그 수정. 기존 `callClaude*` 메서드는 경로 A(GUI 승인 플로우)용으로 유지 |
| `gui/internal/agent.go` | `LogPath` 필드, `LogEntry` 구조체, JSONL 듀얼 라이트, `filterEnv()` 추가 |
| `gui/internal/workspace.go` | `JobsDir`, `LogsDir` 필드, `BuildAgentsFromPlans()` 공유 팩토리, `FindWorkspaceRoot()`, `SessionPath()/LoadSession()/SaveSession()` 추가 |
| `gui/app.go` | `RunLeadSession()` GUI 바인딩 (LogWatcher 통합) 추가 |

## 2.8 두 실행 경로의 공존

```
경로 A (GUI 승인 플로우) — 기존 유지
    PlanRequest → 사용자 승인 → ExecutePlan
    lead.go의 Decompose/GenerateContract/summarize 등 사용
    Claude 호출: Lead 6~8회 + sub-agent N회

경로 B (단일 세션 모드) — v2.1 신규
    RunLeadSession → claudestra CLI → assign × N
    Claude 호출: Lead 1회 + sub-agent N회
```

GUI는 사용자 승인이 필요한 경로 A를 사용하고, CLI는 자율 실행 경로 B를 사용합니다.
향후 경로 A도 토큰 최적화가 필요하면 `Decompose + GenerateContract`를 하나로 합치는 방식으로 개선 가능합니다.

---

# 3. 시스템 아키텍처 (현재 구현)

## 3.1 프로젝트 구조

```
Claudestra/
├── src/                          ← Python MVP (Phase 1)
│   ├── main.py
│   ├── agent.py
│   ├── lead_agent.py
│   └── workspace.py
│
├── gui/                          ← Go + Wails GUI (Phase 3, 현재)
│   ├── main.go                   ← Wails 앱 진입점
│   ├── app.go                    ← GUI ↔ Go 바인딩 (PlanRequest, ExecutePlan, RunLeadSession 등)
│   ├── cmd/claudestra/
│   │   └── main.go               ← claudestra CLI 바이너리 진입점 (v2.1)
│   ├── internal/
│   │   ├── lead.go               ← 팀장 에이전트 (Decompose, Contract, RunLeadSession 등)
│   │   ├── agent.go              ← 팀원 에이전트 (subprocess 래핑, 스트리밍, JSONL 듀얼 라이트)
│   │   ├── streamparser.go       ← 공유 Claude CLI stream-json 파서
│   │   ├── workspace.go          ← 디렉토리 격리, 이데아/설정, BuildAgentsFromPlans 팩토리
│   │   ├── filelock.go           ← 파일 락 레지스트리
│   │   ├── jobs.go               ← Job CRUD (비동기 assign용) (v2.1)
│   │   └── logwatcher.go         ← fsnotify 기반 JSONL 로그 감시 (v2.1)
│   └── frontend/src/
│       ├── App.tsx               ← 메인 앱 (상태 관리, 이벤트 수신)
│       └── components/
│           ├── LogPanel.tsx       ← 로그 패널 (thinking 그룹, 스트리밍)
│           ├── ProposalPanel.tsx  ← 실행 계획 검토 패널
│           ├── Sidebar.tsx        ← 에이전트 목록
│           ├── InputBar.tsx       ← 사용자 입력
│           ├── ReportPanel.tsx    ← 최종 보고서
│           ├── AgentDetailPanel.tsx ← 에이전트 상세 정보
│           └── ProjectSetup.tsx   ← 프로젝트 초기화
│
└── docs/                         ← 기술 문서
```

## 3.2 워크스페이스 디렉토리 구조

```
ProjectDir/                       ← 팀장(Lead) playground
│
├── .orchestra/                   ← Claudestra 내부 설정
│   ├── config.yaml               ← 프로젝트 설정
│   ├── ideas/                    ← 에이전트별 이데아
│   │   ├── vision_dev.yaml
│   │   └── build_qa.yaml
│   ├── session.json              ← 프로젝트 메모리 (세션 상태)
│   ├── team.json                 ← 동적 팀 구성 정보
│   ├── contracts/
│   │   └── contract.yaml         ← 인터페이스 계약서
│   ├── locks/                    ← 파일 락 레지스트리
│   ├── jobs/                     ← 비동기 assign job 파일 (v2.1)
│   │   └── job-{id}.json
│   └── logs/                     ← 에이전트 JSONL 실시간 로그 (v2.1)
│       └── {agent}.jsonl
│
├── vision_dev/                   ← Producer 팀원 playground
│   └── .agent-status
│
└── build_qa/                     ← Consumer 팀원 playground
    └── .agent-status
```

## 3.3 에이전트 유형 분류

| 유형 | 예시 역할 | 허용 도구 | 크로스 참조 |
|------|-----------|-----------|------------|
| Producer | developer, vision_dev | Read, Write, Edit, Glob, Grep, Bash | 없음 |
| Consumer | reviewer, build_qa | Read, Glob, Grep (+ Write 선택적) | 타 Producer dirs 읽기 전용 |
| Lead | 팀장 | Read, Glob, Grep, Bash (v2.0에서 Claudestra CLI 포함) | 전체 |

---

# 4. 실시간 스트리밍 아키텍처

## 4.1 Claude CLI stream-json 와이어 포맷

`claude -p --output-format stream-json --include-partial-messages --verbose` 명령의 실제 출력:

```
{"type":"system","subtype":"init","model":"...","tools":[...]}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"분석 중..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"응답 텍스트..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"result","subtype":"success","result":"최종 텍스트","duration_ms":...}
```

**핵심 포인트:**
- `--include-partial-messages` 없이는 `stream_event` 이벤트가 발생하지 않음
- `--verbose` 없이는 `stream-json` 출력 자체가 에러
- `thinking_delta`의 텍스트는 `delta.thinking` 필드에, `text_delta`는 `delta.text` 필드에 있음

## 4.2 공유 스트림 파서 (streamparser.go)

```go
type StreamCallbacks struct {
    OnText     func(text string)                   // 단어 단위 flush
    OnThinking func(text string)                   // thinking 단어 단위 flush
    OnToolUse  func(toolName string, input string) // 도구 호출 요약
    OnResult   func(result string)                 // 최종 결과
}

func ParseStream(reader io.Reader, cb StreamCallbacks)
```

**flush 전략:** 공백/줄바꿈 포함 토큰 도착 시 즉시 flush → 단어 단위 스트리밍.

## 4.3 이벤트 흐름

```
Claude CLI stdout
    │
    ▼
ParseStream (streamparser.go)
    │
    ├─ OnThinking("분석 중...")  → logFn("💭 분석 중...")
    ├─ OnToolUse("Read", "file.go") → logFn("🔧 Read: file.go")
    ├─ OnText("응답 텍스트...")  → logFn("응답 텍스트...")
    └─ OnResult("최종")         → fullResult = "최종"
                                         │
                                         ▼
                              app.go logFn 래퍼
                                         │
                              "💭"/"🔧"/"📊" → LogEvent{type:"thinking"}
                              그 외         → LogEvent{type:"text"}
                              "\x01" 접두사  → "log-append" 이벤트 (이전 항목에 이어붙임)
                                         │
                                         ▼
                              LogPanel (thinking 그룹 독립 토글 렌더링)
```

## 4.4 프론트엔드 성능 최적화

| 문제 | 해결 |
|------|------|
| 토큰마다 setState → 초당 수백 회 리렌더링 | `requestAnimationFrame` 배치 → 최대 60회/초 |
| `scrollIntoView({ behavior: 'smooth' })` 큐 폭발 | `behavior: 'auto'` + 사용자 스크롤 시 비활성화 |
| 전체 LogPanel 매번 리렌더링 | `React.memo` 적용 |

---

# 5. 에이전트 메모리 시스템

## 5.1 문제

매번 Claude Code를 새로 실행하면 이전 대화가 사라집니다.
전체 대화를 매번 주입하면 토큰 폭발.

## 5.2 해결 — 구조화된 세션 메모리

```json
// .orchestra/session.json
{
  "project_summary": "카메라 캘리브레이션 시각화 모듈 복구 중",

  "completed_tasks": [
    "vision_visualization_dev: zhangVisualization.h/cpp 파일 복구 완료"
  ],

  "open_issues": [
    {
      "id": "issue-001",
      "found_by": "build_qa",
      "severity": "high",
      "description": "컴파일 에러: DrawLine 함수 시그니처 불일치",
      "file": "zhangVisualization.cpp",
      "status": "open"
    }
  ],

  "recent_conversations": [
    {"role": "user", "content": "시각화 파일 복구해줘"},
    {"role": "lead", "content": "vision_visualization_dev에게 복구 태스크 배분 완료"}
  ]
}
```

## 5.3 토큰 관리 전략

| 항목 | 유지 방법 | 토큰량 |
|------|-----------|--------|
| `project_summary` | 매 작업 후 1~2문장으로 갱신 | 소량 |
| `completed_tasks` | 최근 20개만 요약 보관 | 소량 |
| `open_issues` | 해결되면 `status: resolved` | 중간 |
| `recent_conversations` | 마지막 10턴, 200자 truncate | 제한적 |

## 5.4 세션 갱신 시점

```
assign 완료    → completed_tasks 추가 + project_summary 갱신
reviewer 완료  → open_issues 추가
이슈 수정 완료 → open_issues[n].status → "resolved"
```

> v2.0 도구화 후에는 `claudestra session update`로 팀장 Claude가 직접 갱신.

---

# 6. 에이전트 권한 & 격리 전략

## 6.1 현재 (Phase 3) — `--allowedTools` + 디렉토리 격리

```go
// agent.go
args = append(args, "--allowedTools", strings.Join(a.Config.AllowedTools, ","))
cmd.Dir = a.Config.WorkDir  // 자기 디렉토리로 격리
```

- Producer: `Read, Write, Edit, Glob, Grep, Bash`
- Consumer: `Read, Glob, Grep` (+ 선택적 Write)
- 디렉토리 격리 + 이데아 규칙으로 크로스 접근 방지

## 6.2 Phase 4 — Docker 컨테이너 격리

```go
// Phase 3: subprocess (현재)
cmd := exec.Command("claude", "--print", prompt)
cmd.Dir = agent.WorkDir

// Phase 4: Docker (agent.go의 Start() 내부만 교체)
cmd := exec.Command("docker", "run",
    "--rm",
    "-v", agent.WorkDir+":/workspace",
    "-v", "./.orchestra:/orchestra:ro",
    "--network", "none",
    "--memory", "2g",
    "--cpus", "1.0",
    "claude-agent:latest",
    "claude", "--print", prompt,
)
```

**코드 변경 범위**: `agent.go`의 실행 부분만 교체. 나머지 오케스트레이션 로직은 그대로 유지.

---

# 7. 이데아(Idea) 시스템

각 에이전트의 역할, 전문성, 행동 방침을 정의하는 시스템 프롬프트.
팀장 AI가 `PlanTeam`에서 역할에 맞는 이데아를 자동 생성하며, 사용자가 자유롭게 편집 가능합니다.

```yaml
# 예시: 동적으로 생성된 이데아
role: vision_visualization_dev
idea: |
  당신은 로봇 비전 시각화 개발 전문가입니다.
  담당 디렉토리: ./vision_visualization_dev/

  전문 영역:
  - OpenCV / Qt 기반 이미지 시각화
  - 카메라 캘리브레이션 결과 렌더링
  - KImageColor 캔버스 드로잉

  작업 규칙:
  - 자신의 디렉토리 외부 파일은 절대 수정하지 않습니다.
  - 인터페이스 계약서의 명세를 따릅니다.
```

---

# 8. GUI 워크플로우 (현재)

## 8.1 Plan → Approve → Execute

```
사용자 입력
    │
    ▼
PlanRequest (app.go)
    │
    ├── IsDevTask?  ─── No ──→ DirectReply (팀장 직접 응답)
    │       │
    │      Yes
    │       │
    ├── 팀 없음? ──→ PlanTeam (동적 팀 구성)
    │                    │
    │              buildTeamFromPlans
    │                    │
    ├── Decompose (태스크 분해)
    │       │
    ├── GenerateContract (계약서 생성)
    │       │
    └── ProposalPanel에 계획 표시
             │
             ▼
        사용자 승인/취소
             │
          승인 ──→ ExecutePlan
             │         │
             │    웨이브별 병렬 실행
             │         │
             │    최종 보고서
             │
          취소 ──→ 종료
```

> v2.0 도구화 후에는 이 플로우가 단순화됩니다:
> 팀장 Claude 단일 세션 → `claudestra plan` → 사용자 승인 → `claudestra assign` × N

---

# 9. 개발 단계 로드맵

```
Phase 1  Python MVP          오케스트레이션 핵심 로직 검증          ✅ 완료
Phase 2  권한 제어 강화       --allowedTools 도구 제한 추가          ✅ 완료
Phase 3  Go + Wails GUI       검증된 로직을 Go로 포팅 + GUI         ✅ 완료
  ├── Go 포팅 + React GUI                                          ✅ 완료
  ├── 세션 메모리 (session.json)                                    ✅ 완료
  ├── 동적 팀 구성 (PlanTeam)                                       ✅ 완료
  ├── Plan → Approve → Execute GUI 워크플로우                       ✅ 완료
  ├── 실시간 스트리밍 + Thinking 표시                                ✅ v1.4
  └── 독립 thinking 토글 + 분석 텍스트 그룹화                        ✅ v1.5
Phase 3.5  CLI 도구화          팀장 Claude → Claudestra CLI          ✅ v2.1 완료
  ├── [✅] Phase A: 읽기 전용 CLI (status, team, session, issues, contract, idea, output)
  ├── [✅] Phase B: 동기 assign + JSONL 듀얼 라이트
  ├── [✅] Phase C: 비동기 assign + job 관리 (self-re-exec + Setsid detach)
  ├── [✅] Phase D: Lead 단일 세션 (RunLeadSession + buildLeadSessionPrompt)
  └── [✅] Phase E: GUI fsnotify 통합 (LogWatcher + RunLeadSession GUI 바인딩)
Phase 4  Docker 격리          컨테이너 기반 OS 레벨 샌드박스
Phase 5  Docker Compose       전체 스택 선언적 관리
```

---

# 10. 주요 도전 과제 & 해결 전략

| 도전 과제 | 난이도 | 해결 전략 | 상태 |
|-----------|--------|-----------|------|
| Claude Code stdout 파싱 | 높음 | `stream-json` + `--include-partial-messages` + 공유 파서 | ✅ 해결 |
| 작업 완료 감지 신뢰성 | 중간 | `.agent-status` 파일 + 5분 타임아웃 | ✅ 해결 |
| 팀원 간 파일 충돌 | 중간 | 디렉토리 격리 + FileLockRegistry + 이데아 규칙 | ✅ 해결 |
| **토큰 폭발** | **높음** | **CLI 도구화 + session.json 경량 메모리** | **✅ v2.1** |
| 팀장 태스크 분해 품질 | 높음 | 구조화된 프롬프트 + JSON 출력 강제 + 폴백 로직 | ✅ 해결 |
| Docker 내 Claude 인증 | 중간 | API 키 환경변수 주입 전략 | Phase 4 |

---

# 11. 기술 의존성

**Go (현재):**

| 패키지 | 용도 |
|--------|------|
| `github.com/wailsapp/wails/v2` | GUI 프레임워크 |
| `gopkg.in/yaml.v3` | 이데아/설정 YAML |
| `github.com/fsnotify/fsnotify` | JSONL 로그 파일 감시 (v2.1) |

**프론트엔드:**

| 패키지 | 용도 |
|--------|------|
| React + TypeScript | UI 프레임워크 |

**Python MVP (참고용):**

| 패키지 | 용도 |
|--------|------|
| `pyyaml` | 이데아/설정 YAML 관리 |

---

# 12. 미결 사항

- [x] CLI 도구화 — `claudestra` 바이너리 설계 및 구현 ✅ v2.1
- [x] CLI assign 비동기 실행 — self-re-exec + Setsid detach ✅ v2.1
- [x] GUI ↔ CLI 스트리밍 연동 — fsnotify LogWatcher + JSONL 듀얼 라이트 ✅ v2.1
- [ ] 경로 A 토큰 최적화 — GUI 승인 플로우의 Decompose+GenerateContract 통합
- [ ] `extractIssues`의 Phase D 역할 재정의 — RunLeadSession에서 이슈 추출 담당 결정
- [ ] Docker 이미지 내 Claude Code 인증 처리 방법
- [ ] 에이전트 비용 추적 (Claude Code 사용량 모니터링)
- [ ] 로그 가상화 — 장시간 세션에서 수천 개 로그 항목 렌더링 성능 (react-window 등)
- [ ] 에이전트별 로그 필터링 — 현재 모든 에이전트 로그가 단일 패널에 혼재
