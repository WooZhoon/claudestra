# CLAUDESTRA
## Claude Code Multi-Agent Orchestration System
### 기술 설계 문서 v1.3

| 항목 | 내용 |
|------|------|
| 프로젝트명 | Claudestra |
| MVP 언어 | Python |
| 최종 언어 | Go + React (Wails v2) |
| 대상 플랫폼 | Linux / Windows |
| AI 엔진 | Claude Code CLI |
| 에이전트 격리 | subprocess (MVP) → Docker 컨테이너 (최종) |
| 문서 버전 | 1.3.0 |

---

# 1. 프로젝트 개요

## 1.1 목표

Claudestra는 여러 Claude Code 인스턴스를 오케스트레이션하여 복잡한 소프트웨어 개발 작업을 자동화하는 멀티 에이전트 시스템입니다.

사용자는 팀장(Lead Agent)과 다양한 역할의 팀원(Sub-Agent)을 구성하고, 팀장 AI가 사용자의 요구사항을 분석하여 각 팀원에게 최적화된 태스크를 배분합니다.

> **핵심 컨셉**: 팀장 AI = 오케스트레이터 / 팀원 AI = 전문 실행자.
> 각자 격리된 워크스페이스에서 작업하며 팀장이 전체를 조율합니다.

## 1.2 개발 전략 — 단계별 접근

이 프로젝트의 가장 큰 불확실성은 **"Claude Code subprocess를 실제로 안정적으로 제어할 수 있냐"** 입니다.
GUI나 언어 포팅은 그 이후 문제입니다. 따라서 검증 우선 전략을 취합니다.

```
Phase 1  Python MVP          오케스트레이션 핵심 로직 검증        ✅ 완료
Phase 2  권한 제어 강화       --allowedTools 도구 제한 추가        ✅ 완료
Phase 3  Go + Wails GUI       검증된 로직을 Go로 포팅 + GUI      ← 현재
Phase 4  Docker 격리          컨테이너 기반 진짜 샌드박스
Phase 5  Docker Compose       전체 스택 통합 관리
```

## 1.3 주요 특징

- Claude Code CLI를 subprocess로 래핑하여 각 에이전트 독립 실행
- **Plan → Approve → Execute** 워크플로우 — 팀장이 계획 수립 후 사용자 승인을 거쳐 실행
- **인터페이스 계약서(Contract)** 시스템 — 팀원 간 API/DB/네이밍 스펙 통일
- **의존성 기반 태스크 그래프** — `depends_on`으로 의존 관계 명시, 토폴로지 정렬로 자동 병렬화
- **allowedTools 권한 제어** — 역할별 허용 도구 제한 (Producer: 읽기/쓰기, Consumer: 읽기 전용)
- **대화 메모리** — 마지막 보고서 요약을 저장하여 "고쳐줘" 같은 후속 요청 지원
- 역할별 **이데아(Idea)** 시스템 — 사용자 편집 가능한 시스템 프롬프트
- **파일 락 레지스트리** — 에이전트 간 워크스페이스 충돌 방지
- 디렉토리 격리 + 이데아 규칙으로 다단계 방어
- 코드 리뷰어, 문서 작성자 등 크로스 참조(Consumer) 에이전트 지원
- 비개발 입력(인사, 잡담) 자동 감지 — 팀장이 직접 응답하여 불필요한 API 호출 방지
- React Flow 기반 노드 에디터로 팀 구성 시각화 (Phase 3~)
- **최종적으로 Docker 컨테이너 기반 OS 레벨 격리** 적용

## 1.4 기술 스택 요약

| 레이어 | MVP (Python) | 최종 (Go) |
|--------|-------------|-----------|
| 언어 | Python 3.11+ | Go 1.22+ |
| GUI | 없음 (CLI) | Wails v2 + React Flow |
| 에이전트 실행 | subprocess + `--dangerously-skip-permissions` | Docker 컨테이너 |
| 도구 제한 | `--allowedTools` 역할별 설정 | `--allowedTools` + Docker 마운트 |
| 태스크 스케줄링 | 토폴로지 정렬 (Kahn's algorithm) | 토폴로지 정렬 + goroutine |
| 상태 관리 | threading + `.agent-status` 파일 | goroutine + channel |
| 파일 감시 | polling / `.agent-status` | fsnotify |
| 파일 잠금 | FileLockRegistry (threading.Lock + JSON) | FileLockRegistry (sync.RWMutex) |
| 대화 메모리 | `_last_context` (마지막 보고서 5줄 요약) | 동일 구조 |
| 계약서 | YAML (.orchestra/contracts/) | YAML |
| 이데아 저장 | YAML | YAML |
| 의존성 | pyyaml | wails, creack/pty, fsnotify |

---

# 2. 에이전트 권한 & 격리 전략

이 프로젝트에서 가장 중요한 설계 결정 중 하나입니다.
에이전트가 의도치 않게 다른 팀원의 파일이나 시스템을 건드리는 것을 어떻게 막느냐의 문제입니다.

## 2.1 단계별 격리 전략

```
Phase 1 (MVP) ✅        Phase 2 ✅              Phase 4 (최종)
────────────────────────────────────────────────────────────────
--dangerously-         --allowedTools로         Docker 컨테이너
skip-permissions       허용 도구 명시           OS 레벨 격리
+ 디렉토리 격리        + stdin 기반 프롬프트
+ 이데아 규칙          + 역할별 도구 세트
+ 파일 락 레지스트리
+ 인터페이스 계약서
```

## 2.2 Phase 1 — skip-permissions + 다단계 방어 (완료)

MVP 단계에서는 `--dangerously-skip-permissions` 플래그로 permission 프롬프트를 건너뜁니다.
대신 네 가지로 리스크를 관리합니다.

**왜 permission 릴레이 방식은 안 쓰나?**

permission 릴레이란 에이전트의 permission 프롬프트를 팀장이 캡처해서 사용자에게 전달하고, 응답을 다시 주입하는 방식입니다.
팀원이 5명이면 동시다발로 permission이 터지고, UX도 엉망이 되며, 구현 복잡도만 높아집니다. 실용적이지 않습니다.

**대신 이 네 가지로 방어합니다:**

1. **WorkDir 격리** — 각 에이전트의 작업 디렉토리를 자신의 서브 디렉토리로 강제 지정
2. **이데아 규칙** — "자신의 디렉토리 외부 파일 수정 금지"를 시스템 프롬프트에 명시
3. **파일 락 레지스트리** — 에이전트 실행 시 디렉토리 잠금 획득, 충돌 시 실행 차단
4. **인터페이스 계약서** — 팀장이 공통 스펙을 생성하여 모든 에이전트에 주입

```python
# 1. WorkDir 격리
subprocess.run(
    ["claude", "--print", "--dangerously-skip-permissions", prompt],
    cwd=str(agent.work_dir),   # ← 자신의 서브 디렉토리로 강제
)

# 2. 이데아 규칙 (시스템 프롬프트에 포함)
# "자신의 디렉토리(./backend/) 외부 파일은 절대 수정하지 않습니다."

# 3. 파일 락 (실행 전 자동 획득, 완료 후 자동 해제)
lock_registry.acquire(str(agent.work_dir), agent.agent_id)
# ... 작업 실행 ...
lock_registry.release_all(agent.agent_id)  # finally 블록에서 해제

# 4. 계약서 (프롬프트에 주입)
# "[인터페이스 계약서 — 반드시 이 명세를 따르세요]"
```

## 2.3 Phase 2 — allowedTools 도구 제한 (완료)

`--allowedTools` 플래그로 에이전트가 사용할 수 있는 도구를 역할별로 제한합니다.

**역할별 허용 도구:**

| 역할 | 허용 도구 | 설명 |
|------|-----------|------|
| backend, frontend, db | Read, Write, Edit, Glob, Grep, Bash | 코드 작성 가능 |
| reviewer | Read, Glob, Grep | 읽기 전용 — 코드 수정 불가 |
| doc_writer | Read, Glob, Grep, Write | 문서 작성만 허용 |

```python
# main.py — 역할별 도구 매핑
PRODUCER_TOOLS = ["Read", "Write", "Edit", "Glob", "Grep", "Bash"]
CONSUMER_TOOLS = ["Read", "Glob", "Grep"]

ROLE_TOOLS = {
    "backend":    PRODUCER_TOOLS,
    "frontend":   PRODUCER_TOOLS,
    "db":         PRODUCER_TOOLS,
    "reviewer":   CONSUMER_TOOLS,
    "doc_writer": CONSUMER_TOOLS + ["Write"],
}
```

**stdin 기반 프롬프트 전달:**

`--allowedTools`는 가변 인자(`<tools...>`)로, 뒤에 오는 위치 인자를 도구 이름으로 먹어버리는 문제가 있습니다.
이를 해결하기 위해 프롬프트를 위치 인자 대신 **stdin**으로 전달합니다.

```python
# Before (충돌 발생):
cmd = ["claude", "--print", "--allowedTools", "Read,Write", prompt]  # ← prompt가 도구로 인식됨

# After (stdin으로 분리):
cmd = ["claude", "--print", "--allowedTools", "Read,Write"]
subprocess.run(cmd, input=prompt, ...)  # ← prompt는 별도 채널
```

## 2.4 Phase 4 — Docker 컨테이너 격리 (최종)

최종 단계에서 각 에이전트는 독립된 Docker 컨테이너로 실행됩니다.
subprocess 방식과 달리 OS 레벨에서 완전히 격리됩니다.

```
현재 (subprocess)                  최종 (Docker)
─────────────────────────────────────────────────────
에이전트가 이데아 무시하면          컨테이너 경계로
→ ../다른팀원/ 건드릴 수 있음       → 물리적으로 불가능
→ 시스템 파일 건드릴 수 있음        → 마운트된 경로만 접근
→ 네트워크 마음대로 사용            → 네트워크 격리
```

**Docker 실행 구조:**

```bash
docker run \
  --rm \
  -v ./backend:/workspace \       # 자기 서브 디렉토리만 마운트
  -v ./.orchestra:/orchestra:ro \ # 설정 읽기 전용 (계약서 포함)
  --network none \                 # 네트워크 완전 차단
  --memory 2g \                    # 메모리 제한
  --cpus 1.0 \                     # CPU 제한
  claude-agent:latest \
  claude --print "{prompt}"
```

**코드 변경 범위 (subprocess → Docker):**

Go 코드 기준으로 `agent.go`의 `Start()` 함수 내부만 교체하면 됩니다.
나머지 오케스트레이션 로직(계획 수립, 계약서, 잠금 등)은 그대로 유지됩니다.

```go
// Phase 1~3: subprocess
cmd := exec.Command("claude", "--print", prompt)
cmd.Dir = agent.WorkDir

// Phase 4: Docker (이 부분만 교체)
cmd := exec.Command("docker", "run",
    "--rm",
    "-v", agent.WorkDir+":/workspace",
    "-v", agent.ContractsDir+":/orchestra/contracts:ro",
    "--network", "none",
    "--memory", "2g",
    "claude-agent:latest",
    "claude", "--print", prompt,
)
```

---

# 3. 시스템 아키텍처

## 3.1 전체 구조도

```
┌─────────────────────────────────────────────────────┐
│                   GUI Layer (Wails)   ← Phase 3~     │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │  Node Editor  │  │ Idea Editor  │  │  Log View │  │
│  │ (React Flow)  │  │  (textarea)  │  │ (stream)  │  │
│  └──────┬────────┘  └──────┬───────┘  └─────┬─────┘  │
└─────────┼──────────────────┼────────────────┼────────┘
          │                  │                │
┌─────────▼──────────────────▼────────────────▼────────┐
│            Orchestration Engine                       │
│         (Python MVP → Go 최종)                        │
│  ┌─────────────────────────────────────────────────┐  │
│  │              AgentManager                        │  │
│  │  ┌────────────┐    ┌────────────────────────┐   │  │
│  │  │ LeadAgent  │───▶│  Plan-Approve-Execute   │   │  │
│  │  │ + Memory   │    │  + Contract Generator   │   │  │
│  │  └────────────┘    └──────────┬─────────────┘   │  │
│  │                               │                  │  │
│  │  ┌────────────────────────────▼───────────────┐  │  │
│  │  │       Dependency Graph (Toposort)           │  │  │
│  │  │  t1(db,[]) → t2(be,[t1]) + t3(fe,[t1])    │  │  │
│  │  │              → t4(reviewer,[t2,t3])         │  │  │
│  │  └────────────────────────────────────────────┘  │  │
│  │                               │                  │  │
│  │  ┌────────────────────────────▼───────────────┐  │  │
│  │  │           SubAgent Pool                     │  │  │
│  │  │  [Backend] [Frontend] [DB] [Doc] [Reviewer] │  │  │
│  │  │  +allowedTools per role                     │  │  │
│  │  └────────────────────────────────────────────┘  │  │
│  │                                                   │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │  FileLockRegistry    ContractStore           │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  └─────────────────────────────────────────────────┘  │
└───────────────────────────┬───────────────────────────┘
                            │
         ┌──────────────────┴──────────────────┐
         │                                     │
┌────────▼────────┐                  ┌─────────▼───────┐
│  Phase 1~3      │                  │  Phase 4~5       │
│  subprocess     │                  │  Docker          │
│  + stdin prompt │                  │  컨테이너         │
│  + PTY (Go)     │                  │                   │
└─────────────────┘                  └─────────────────┘
```

## 3.2 핵심 워크플로우: Plan → Approve → Execute

```
사용자 입력: "계산기 만들어줘"
    │
    ▼
┌─────────────────────────────────────┐
│  1. 계획 수립 (_decompose)           │
│     팀장 AI가 의존성 기반 태스크 생성  │
│     → depends_on으로 선후 관계 명시   │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  1.5 토폴로지 정렬 (_toposort)       │
│     Kahn's algorithm으로 실행 웨이브  │
│     자동 생성 (병렬화 최적화)         │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  2. 계약서 생성 (_generate_contract) │
│     기술스택, API 명세, DB 스키마,    │
│     네이밍 규칙 등 공통 스펙 작성     │
│     → .orchestra/contracts/ 에 저장  │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  3. 사용자에게 계획+계약서 표시      │
│     "진행할까요?"                    │
│     y: 전체 실행                     │
│     s: 단계별 확인 (단계마다 y/n)    │
│     n: 취소                          │
└──────────────┬──────────────────────┘
               │ 승인
               ▼
┌─────────────────────────────────────┐
│  4. 계약서를 모든 에이전트에 주입     │
│     agent.config.contract = contract │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  5. 웨이브별 실행 (토폴로지 순서)    │
│     Wave 1: [DB] 의존성 없음 → 실행  │
│        ↓ 완료                        │
│     Wave 2: [Backend] [Frontend] 병렬│
│        ↓ 완료                        │
│     Wave 3: [Reviewer] 실행          │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│  6. 최종 보고서 작성 (_summarize)    │
│     + 보고서 요약을 메모리에 저장     │
└─────────────────────────────────────┘
```

**비개발 입력 처리:**

"안녕?", "고마워" 같은 비개발 입력은 팀장이 빈 태스크 배열(`[]`)을 반환하고 직접 응답합니다.
에이전트를 동원하지 않으므로 API 호출 1회로 처리됩니다 (기존 6회 → 1회).

단, 이전 작업 컨텍스트가 있고 "고쳐줘", "수정해" 같은 참조 입력인 경우에는 개발 태스크로 분류합니다.

## 3.3 워크스페이스 디렉토리 구조

```
MainWorkspace/                    ← 팀장(Lead) playground
│
├── .orchestra/                   ← Claudestra 내부 설정
│   ├── config.yaml               ← 프로젝트 설정 (등록된 에이전트 목록)
│   ├── ideas/                    ← 에이전트별 이데아 (사용자 편집 가능)
│   │   ├── backend.yaml
│   │   ├── frontend.yaml
│   │   └── reviewer.yaml
│   ├── contracts/                ← 인터페이스 계약서
│   │   └── contract.yaml         ← 현재 활성 계약서
│   └── locks/                    ← 파일 락 레지스트리
│       └── registry.json         ← 현재 잠금 상태
│
├── backend/                      ← Backend 팀원 playground
│   ├── src/
│   └── .agent-status
│
├── frontend/                     ← Frontend 팀원 playground
│   ├── components/
│   └── .agent-status
│
├── db/                           ← DB 팀원 playground
│   ├── migrations/
│   └── .agent-status
│
├── doc-writer/                   ← 문서 작성자 (Consumer)
│   ├── docs/
│   └── .agent-status
│
└── code-reviewer/                ← 코드 리뷰어 (Consumer)
    ├── reviews/
    └── .agent-status
```

## 3.4 에이전트 유형 분류

| 유형 | 예시 역할 | 워크스페이스 권한 | 허용 도구 | 크로스 참조 | Docker 마운트 |
|------|-----------|------------------|-----------|------------|--------------|
| Producer | Backend, Frontend, DB | 자신의 서브 dir 읽기/쓰기 | Read,Write,Edit,Glob,Grep,Bash | 없음 | 자기 dir만 rw |
| Consumer | Code Reviewer, Doc Writer | 자신의 서브 dir 읽기/쓰기 | Read,Glob,Grep (읽기전용) | 다른 서브 dirs 읽기 전용 | 자기 dir rw + 타 dirs ro |
| Lead | 팀장 (Orchestrator) | 메인 dir 전체 읽기/쓰기 | 제한 없음 | 모든 dirs | 전체 rw |

---

# 4. 핵심 컴포넌트 설계

## 4.1 파일 구성 (Python MVP)

```
Claudestra/
├── src/
│   ├── main.py           ← CLI 진입점 (init / run / idea / status) + 역할별 도구 매핑
│   ├── lead_agent.py     ← 팀장 AI (계획 수립, 토폴로지 정렬, 계약서 생성, 대화 메모리, 보고)
│   ├── agent.py          ← 에이전트 기반 클래스 (subprocess 래핑 + 잠금 + allowedTools)
│   ├── workspace.py      ← 워크스페이스, 이데아, 계약서 관리
│   └── file_lock.py      ← 파일 락 레지스트리
├── docs/                 ← 설계 문서
├── Dockerfile            ← Docker 테스트 환경
├── run.sh                ← Docker 빌드 & 실행 스크립트
└── .gitignore
```

## 4.2 AgentConfig & Agent (Python)

```python
@dataclass
class AgentConfig:
    agent_id:       str
    role:           str
    idea:           str          # 시스템 프롬프트 (이데아)
    work_dir:       Path
    read_refs:      list[Path]   # Consumer 전용 읽기 전용 참조 경로
    contract:       str = ""     # 인터페이스 계약서 (팀장이 주입)
    allowed_tools:  list[str]    # 허용 도구 목록 (빈 = 제한 없음)

class Agent:
    def __init__(self, config: AgentConfig, lock_registry=None):
        """lock_registry가 주어지면 실행 시 자동 잠금"""

    def run(self, instruction: str) -> str:
        """blocking 실행 — 잠금 획득 → --allowedTools로 제한된 작업 → 잠금 해제"""

    def run_async(self, instruction: str) -> threading.Thread:
        """비동기 실행 — 스레드 반환"""

    def _build_prompt(self, instruction: str) -> str:
        """이데아 + 작업 원칙 + 계약서 + 크로스 참조 + 지시를 결합"""
```

**에이전트 실행 구조 (stdin 기반):**

```python
cmd = ["claude", "--print", "--dangerously-skip-permissions"]
if self.config.allowed_tools:
    cmd += ["--allowedTools", ",".join(self.config.allowed_tools)]

result = subprocess.run(
    cmd,
    input=prompt,           # ← stdin으로 프롬프트 전달
    cwd=str(self.config.work_dir),
    capture_output=True,
    text=True,
    timeout=300,
)
```

**에이전트 프롬프트 구조:**

```
[이데아] 당신은 백엔드 개발 전문가입니다...

[작업 원칙]
- 간결하게 핵심만 작성하세요.
- 핵심 구조, 주요 파일, 중요 로직만 구현하세요.

[인터페이스 계약서 — 반드시 이 명세를 따르세요]
tech_stack:
  backend: Express.js
  db: PostgreSQL
api_endpoints:
  - POST /api/auth/login
  ...

[읽기 전용 참조 디렉토리 — 수정 금지]   ← Consumer만
- ./backend/
- ./frontend/

[지시] 사용자 인증 API를 구현하세요...
```

## 4.3 이데아(Idea) 시스템

이데아는 각 에이전트의 역할, 전문성, 행동 방침을 정의하는 시스템 프롬프트입니다.
팀장 AI가 역할에 맞는 기본 이데아를 자동 생성하며, 사용자가 자유롭게 편집할 수 있습니다.

> **편집 방법**: `python main.py idea -p ./my-project`

```yaml
# .orchestra/ideas/backend.yaml
role: backend
idea: |
  당신은 백엔드 개발 전문가입니다.
  담당 디렉토리: ./backend/

  전문 영역:
  - RESTful API 설계 및 구현
  - 인증/인가 시스템 (JWT, OAuth2)
  - 성능 최적화 및 캐싱 전략

  작업 규칙:
  - 자신의 디렉토리(./backend/) 외부 파일은 절대 수정하지 않습니다.
  - 작업 완료 시 .agent-status 파일에 DONE을 기록합니다.
  - 인터페이스 계약서가 제공되면 반드시 그 명세를 따릅니다.
```

## 4.4 인터페이스 계약서(Contract) 시스템

팀원 간 인터페이스 불일치를 방지하는 핵심 기능입니다.

**문제:** 격리된 에이전트들이 각자 독립적으로 네이밍, API 스펙, DB 엔진을 결정하면
백엔드는 `teacher`, 프론트는 `instructor`를 쓰는 등 통합 불가능한 결과물이 나옴.

**해결:** 팀장이 계획 수립 직후, 실행 전에 공통 계약서를 생성하여 모든 에이전트에 주입.

```yaml
# .orchestra/contracts/contract.yaml (팀장이 자동 생성)
tech_stack:
  language: TypeScript
  backend: Express.js
  frontend: React Native
  database: PostgreSQL

naming_conventions:
  fields: snake_case
  api_paths: kebab-case
  identifiers: English only

api_endpoints:
  - method: POST
    path: /api/auth/login
    request: { email: string, password: string }
    response: { access_token: string, refresh_token: string }

db_schema:
  students:
    - id: serial PRIMARY KEY
    - name: varchar(100)
    - parent_id: integer REFERENCES parents(id)

shared_types:
  Student: { id, name, grade, parent_id, created_at }
  Teacher: { id, name, subject, phone }
```

## 4.5 파일 락 레지스트리

에이전트 간 워크스페이스 충돌을 방지하는 2차 방어 메커니즘입니다.

```python
class FileLockRegistry:
    """Thread-safe 파일 잠금 레지스트리"""

    def acquire(self, file_path, agent_id):
        """잠금 획득. 다른 에이전트가 보유 중이면 LockConflictError"""

    def release_all(self, agent_id):
        """에이전트의 모든 잠금 해제 (작업 완료 후)"""

    def list_locks(self):
        """현재 잠금 상태 조회 (status 명령에서 사용)"""
```

잠금 상태는 `.orchestra/locks/registry.json`에 저장되어 `status` 명령으로 확인 가능합니다.

## 4.6 의존성 기반 태스크 그래프 & 토폴로지 정렬

v1.3에서 step 기반 태스크 포맷을 **의존성 그래프(DAG)**로 교체했습니다.

**Before (v1.2) — 팀장이 step 번호를 수동 지정:**
```json
[
  {"step": 1, "title": "DB 설계", "tasks": [{"agent_id": "db", ...}]},
  {"step": 2, "title": "구현", "tasks": [{"agent_id": "backend", ...}, {"agent_id": "frontend", ...}]}
]
```

**After (v1.3) — 팀장이 의존 관계만 명시, 시스템이 자동 병렬화:**
```json
[
  {"id": "t1", "agent_id": "db",       "instruction": "테이블 설계",    "depends_on": []},
  {"id": "t2", "agent_id": "backend",  "instruction": "API 구현",      "depends_on": ["t1"]},
  {"id": "t3", "agent_id": "frontend", "instruction": "UI 구현",       "depends_on": ["t1"]},
  {"id": "t4", "agent_id": "reviewer", "instruction": "코드 리뷰",     "depends_on": ["t2", "t3"]}
]
```

**토폴로지 정렬 (Kahn's algorithm):**

```
입력 그래프:
  t1 → t2 → t4
  t1 → t3 ↗

자동 변환 결과:
  Wave 1: [t1(db)]                ← in_degree=0
  Wave 2: [t2(backend), t3(frontend)] ← t1 완료 후, 둘 다 병렬
  Wave 3: [t4(reviewer)]              ← t2,t3 모두 완료 후
```

**장점:**
- 팀장 AI가 병렬화를 직접 결정할 필요 없음 — 의존 관계만 명시하면 시스템이 최적 배치
- 순환 의존성 자동 감지 + 경고 로그 출력
- 변환 결과가 기존 step 포맷과 동일하므로 실행 로직 변경 없음

**순환 의존성 처리:**

```python
# 방문하지 못한 노드가 있으면 순환 의존성
if len(visited) < len(valid_ids):
    orphans = valid_ids - visited
    print(f"[팀장] ⚠️  순환 의존성 감지: {orphans}")
    waves.append([task_map[tid] for tid in orphans])  # 마지막 웨이브에 강제 추가
```

## 4.7 대화 메모리

v1.3에서 추가된 기능입니다. 팀장이 이전 실행 결과를 기억하여 후속 요청을 처리합니다.

**문제:** `claude --print`는 매번 새 프로세스라 이전 대화를 모름.
"응 고쳐줘" 같은 입력이 비개발로 분류되거나, 뭘 고쳐야 하는지 모름.

**해결:** 보고서 작성 후 핵심 요약(5줄 이내)만 저장, 다음 요청 시 컨텍스트로 주입.

```
보고서 → _save_context() → Claude에게 5줄 요약 요청 → _last_context에 저장
                                                          ↓
다음 요청 → _build_context_block() → [이전 작업 컨텍스트] 블록으로 주입
                                          ↓
                          _decompose() 및 _direct_reply() 프롬프트에 포함
```

**토큰 비용:** 요약 호출 1회 (~100 토큰 출력) + 다음 요청 시 ~100-200 토큰 주입.
전체 보고서(~2000 토큰)를 매번 주입하는 것 대비 약 1/10 비용.

**컨텍스트 주입 예시:**
```
[이전 작업 컨텍스트 — 사용자가 이전 작업을 참조할 수 있습니다]
계산기 웹앱 구현 완료. 해결 필요: 1) X-Hidden-Access 헤더 미전송 (CRITICAL)
2) Vite 프록시 포트 불일치 3) package.json 진입점 오류.
다음 단계: P0 이슈 4건 수정 후 통합 테스트.
```

## 4.8 AgentNode 구조체 (Go — Phase 3~)

```go
type AgentNode struct {
    ID           string
    Role         string
    RoleType     AgentRoleType    // Producer | Consumer | Lead
    WorkDir      string
    Idea         string
    Contract     string           // 인터페이스 계약서
    AllowedTools []string         // 허용 도구 목록

    // Phase 1~3: subprocess
    Process     *os.Process
    PTY         *os.File

    // Phase 4~: Docker
    ContainerID string

    // State
    Status      AgentStatus
    CurrentTask *Task
    OutputChan  chan string
    DoneChan    chan struct{}

    ReadRefs    []string         // Consumer 전용
    mu          sync.RWMutex
}

// 태스크 의존성 그래프
type TaskGraph struct {
    Tasks      map[string]*Task         // id → task
    Dependents map[string][]string      // id → 이 태스크에 의존하는 태스크들
    InDegree   map[string]int           // id → 진입 차수
}

func (g *TaskGraph) TopoSort() [][]Task  // 실행 웨이브 반환
```

---

# 5. 워크스페이스 충돌 방지

## 5.1 5단계 방어 전략

| 단계 | 방법 | 적용 시점 | 설명 |
|------|------|-----------|------|
| 1차 | 디렉토리 격리 (WorkDir 고정) | Phase 1~ (항상) | 각 에이전트 cwd를 서브 디렉토리로 강제 |
| 2차 | 이데아 규칙 (외부 수정 금지 명시) | Phase 1~ (항상) | 시스템 프롬프트로 행동 제한 |
| 3차 | 파일 락 레지스트리 | Phase 1~ (구현됨) | 동일 디렉토리 동시 접근 차단 |
| 4차 | 인터페이스 계약서 | Phase 1~ (구현됨) | 공통 스펙으로 결과물 호환성 보장 |
| 5차 | allowedTools 도구 제한 | Phase 2~ (구현됨) | 역할별 CLI 도구 접근 제한 |
| 최종 | Docker 컨테이너 경계 | Phase 4~ | OS 레벨 물리적 격리 |

## 5.2 .agent-status 완료 감지

각 에이전트는 작업 완료 시 자신의 디렉토리에 `.agent-status` 파일을 씁니다.

```
IDLE     → 대기 중
RUNNING  → 작업 중
DONE     → 완료
ERROR    → 오류 발생
```

**Python MVP:** threading + join 방식으로 완료 대기
**Go 단계:** fsnotify로 이벤트 기반 감지

## 5.3 크로스 참조 에이전트 (Consumer)

코드 리뷰어, 문서 작성자는 Producer들이 모두 완료된 후 실행됩니다.
토폴로지 정렬에 의해 자동으로 후순위에 배치됩니다 (depends_on으로 Producer 태스크를 참조).

```python
# Consumer 프롬프트에 자동 추가
[읽기 전용 참조 디렉토리 — 수정 금지]
- ./backend/
- ./frontend/
- ./db/
```

---

# 6. GUI 설계 (Phase 3~ / Wails v2 + React Flow)

## 6.1 주요 화면 구성

| 화면 | 설명 | 주요 기능 |
|------|------|-----------|
| Node Editor | 에이전트 노드 시각화 | 노드 추가/삭제, 연결선, 드래그 |
| Idea Editor | 이데아 편집 패널 | 노드 클릭 시 열리는 텍스트 편집기 |
| Contract Viewer | 계약서 편집/조회 | 현재 활성 계약서 표시 및 편집 |
| Task Graph | 태스크 의존성 그래프 | DAG 시각화, 실행 중인 웨이브 하이라이트 |
| Log Viewer | 실시간 에이전트 출력 | 에이전트별 로그 스트리밍 |
| Workspace Explorer | 디렉토리 구조 탐색 | 파일 트리, 잠금 상태 표시 |

## 6.2 Wails 백엔드 바인딩 (Go)

```go
func (a *App) AddAgent(role string, roleType string) (string, error)
func (a *App) UpdateIdea(agentID string, idea string) error
func (a *App) SubmitUserInput(input string) error
func (a *App) GetContract() (string, error)
func (a *App) UpdateContract(contract string) error
func (a *App) GetLocks() (map[string]string, error)
func (a *App) GetTaskGraph() (*TaskGraph, error)  // 태스크 DAG 조회

// 실시간 로그 스트리밍
func (a *App) streamLogs(agentID string, ch chan string) {
    for log := range ch {
        runtime.EventsEmit(a.ctx, "log:"+agentID, log)
    }
}
```

---

# 7. 개발 단계 로드맵

## Phase 1 — Python MVP ✅ 완료

> **목표**: 오케스트레이션 핵심 로직 검증. GUI 없음.

**구현 완료:**
```
✅ 팀장 1명 + 팀원 N명 구성
✅ subprocess Claude Code 실행
✅ 디렉토리 격리
✅ Plan → Approve → Execute 워크플로우
✅ 인터페이스 계약서 시스템
✅ 파일 락 레지스트리
✅ 비개발 입력 자동 감지 & 팀장 직접 응답
✅ .agent-status 완료 감지
✅ 이데아 YAML 편집 (CLI)
✅ Docker 테스트 환경 (Dockerfile + run.sh)
```

## Phase 2 — 권한 제어 강화 ✅ 완료

> **목표**: `--allowedTools`로 에이전트 도구 접근 제한

**구현 완료:**
```
✅ --allowedTools 플래그 적용 (역할별 도구 세트)
✅ Producer / Consumer 별 허용 도구 분리
✅ stdin 기반 프롬프트 전달 (가변인자 충돌 해결)
✅ 의존성 기반 태스크 그래프 + 토폴로지 정렬
✅ 대화 메모리 (마지막 보고서 요약 저장)
✅ 순환 의존성 감지
```

## Phase 3 — Go + Wails GUI ← 현재

> **목표**: 검증된 Python 로직을 Go로 포팅 + GUI 추가

1. Go 프로젝트 초기화 (Wails v2 scaffolding)
2. Python 로직 → Go 포팅 (AgentNode, LeadAgent, Workspace, FileLock, Contract, TaskGraph)
3. PTY subprocess 래핑 (`creack/pty`)
4. React Flow 노드 에디터
5. 이데아/계약서 편집 사이드패널
6. 태스크 DAG 시각화
7. 실시간 로그 스트리밍 (Wails EventsEmit)
8. fsnotify 기반 완료 감지 전환

## Phase 4 — Docker 컨테이너 격리

> **목표**: subprocess → Docker 교체로 OS 레벨 샌드박스 구현

1. `claude-agent` Docker 이미지 작성
2. `agent.go`의 `Start()` 함수 내부만 Docker 실행으로 교체
3. 볼륨 마운트 전략 (rw / ro) 적용 — 계약서 디렉토리 포함
4. 네트워크 격리 (`--network none` 또는 커스텀 네트워크)
5. 리소스 제한 (`--memory`, `--cpus`)
6. 컨테이너 생명주기 관리

## Phase 5 — Docker Compose 통합

> **목표**: 전체 스택을 Docker Compose로 선언적 관리

```yaml
# docker-compose.yml 예시
services:
  lead:
    image: claude-agent:latest
    volumes:
      - ./workspace:/workspace
  backend:
    image: claude-agent:latest
    volumes:
      - ./workspace/backend:/workspace
      - ./workspace/.orchestra/contracts:/contracts:ro
    network_mode: none
  frontend:
    image: claude-agent:latest
    volumes:
      - ./workspace/frontend:/workspace
      - ./workspace/.orchestra/contracts:/contracts:ro
    network_mode: none
```

---

# 8. 주요 도전 과제 & 해결 전략

| 도전 과제 | 난이도 | 해결 전략 | 현재 상태 |
|-----------|--------|-----------|-----------|
| Claude Code stdout 파싱 | 높음 | `--print` 모드 + JSON 출력 강제 | ✅ 해결됨 |
| 작업 완료 감지 | 중간 | threading.join (MVP), fsnotify (Go) | ✅ 해결됨 |
| 팀원 간 파일 충돌 | 중간 | 디렉토리 격리 + 이데아 규칙 + 파일 락 | ✅ 해결됨 |
| 팀원 간 인터페이스 불일치 | 높음 | 인터페이스 계약서 자동 생성 & 주입 | ✅ 해결됨 |
| 태스크 분해 & 병렬화 | 높음 | 의존성 DAG + 토폴로지 정렬 + 폴백 | ✅ 해결됨 |
| 불필요한 API 호출 | 중간 | 비개발 입력 감지 → 팀장 직접 응답 | ✅ 해결됨 |
| 대규모 작업 타임아웃 | 중간 | Plan-Approve-Execute + 간결함 규칙 | ✅ 해결됨 |
| 에이전트 도구 남용 | 중간 | --allowedTools 역할별 제한 + stdin 프롬프트 | ✅ 해결됨 |
| 후속 요청 컨텍스트 부재 | 중간 | 대화 메모리 (보고서 5줄 요약 저장) | ✅ 해결됨 |
| Docker 이미지 Claude Code 설치 | 중간 | Dockerfile에 Node.js + Claude Code CLI 설치 | ✅ 테스트 환경 구현 |
| Windows PTY 지원 | 중간 | `creack/pty` Windows 지원, ConPTY API 폴백 | Phase 3 |

---

# 9. 기술 의존성 목록

**Python MVP:**

| 패키지 | 용도 |
|--------|------|
| `pyyaml` | 이데아/설정 YAML 저장/로드 |

**Go (Phase 3~):**

| 패키지 | 용도 | 라이선스 |
|--------|------|----------|
| `github.com/wailsapp/wails/v2` | GUI 프레임워크 | MIT |
| `github.com/creack/pty` | PTY 지원 | MIT |
| `github.com/fsnotify/fsnotify` | 파일 시스템 감시 | BSD-3 |
| `github.com/google/uuid` | 에이전트 고유 ID | BSD-3 |
| `gopkg.in/yaml.v3` | 이데아 YAML | Apache-2.0 |
| `reactflow` | 노드 에디터 UI | MIT |
| `zustand` | React 상태 관리 | MIT |

---

# 10. 미결 사항 & 추후 논의

- Claude Code CLI의 `--output-format stream-json` 정식 지원 여부 확인
- Docker 이미지 내 Claude Code 인증 처리 방법 (API 키 주입 vs OAuth 마운트)
- 계약서 포맷 고도화 — 현재 자유형 YAML에서 JSON Schema/OpenAPI 기반으로 전환 검토
- 멀티 세션 저장/복원 전략 — 프로젝트 단위 vs 세션 단위
- 에이전트 비용 추적 — Claude Code 사용량 모니터링
- Docker Compose vs Kubernetes (규모에 따라)
- Windows에서 Docker 성능 이슈 사전 검토 (WSL2 기반)
- 계약서 사용자 편집 기능 — 자동 생성 후 사용자가 수정하고 재실행
- 대화 메모리 확장 — 현재 1턴만 저장, N턴 요약 체인 검토
- `--allowedTools` 경로 패턴 제한 활용 — `Read(./backend/**)` 같은 세밀한 제어
