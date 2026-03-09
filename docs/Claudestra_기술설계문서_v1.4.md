# CLAUDESTRA
## Claude Code Multi-Agent Orchestration System
### 기술 설계 문서 v1.4

| 항목 | 내용 |
|------|------|
| 프로젝트명 | Claudestra |
| MVP 언어 | Python |
| 최종 언어 | Go + React (Wails v2) |
| 대상 플랫폼 | Linux / Windows |
| AI 엔진 | Claude Code CLI |
| 에이전트 격리 | subprocess (MVP) → Docker 컨테이너 (최종) |
| 문서 버전 | 1.4.0 |

---

## v1.4 변경 사항 요약

### 신규 기능: 실시간 스트리밍 & 사고 과정(Thinking) 표시

v1.3까지는 에이전트 실행 결과를 완료 후 일괄 수신했습니다.
v1.4에서는 Claude CLI의 `stream-json` 출력을 실시간 파싱하여 토큰 단위 스트리밍과 사고 과정(Extended Thinking) 표시를 지원합니다.

**주요 변경:**

1. **`--include-partial-messages` 플래그 추가** — 이 플래그 없이는 토큰 단위 스트리밍이 동작하지 않음
2. **공유 스트림 파서 (`streamparser.go`)** — agent.go와 lead.go의 중복 파싱 로직을 통합
3. **Thinking delta 처리** — `thinking_delta` 이벤트를 캡처하여 프론트엔드에 전달
4. **구조화된 로그 이벤트** — `LogEvent{type, message}` 형태로 text/thinking 구분
5. **프론트엔드 성능 최적화** — `requestAnimationFrame` 배치 업데이트, `React.memo`, 스크롤 최적화

---

# 1. 실시간 스트리밍 아키텍처

## 1.1 Claude CLI stream-json 와이어 포맷

`claude -p --output-format stream-json --include-partial-messages --verbose` 명령의 실제 출력:

```
{"type":"system","subtype":"init","model":"...","tools":[...]}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"분석 중..."}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"응답 텍스트..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"assistant","message":{"content":[...]}}
{"type":"result","subtype":"success","result":"최종 텍스트","duration_ms":...}
```

**핵심 포인트:**
- `--include-partial-messages` 없이는 `stream_event` 이벤트가 발생하지 않음
- `--verbose` 없이는 `stream-json` 출력 자체가 에러
- `thinking_delta`의 텍스트는 `delta.thinking` 필드에, `text_delta`는 `delta.text` 필드에 있음
- `result` 이벤트의 `result` 필드에 최종 텍스트가 포함됨

## 1.2 공유 스트림 파서 (streamparser.go)

agent.go와 lead.go에서 중복되던 스트림 파싱 로직을 `internal/streamparser.go`로 통합:

```go
// 와이어 포맷 매핑
type streamEvent struct {
    Type   string          `json:"type"`   // "stream_event", "result", ...
    Event  *streamSubEvent `json:"event"`
    Result string          `json:"result"`
}

type streamSubEvent struct {
    Type         string        `json:"type"` // "content_block_start", "content_block_delta", ...
    ContentBlock *contentBlock `json:"content_block"`
    Delta        *streamDelta  `json:"delta"`
}

type streamDelta struct {
    Type     string `json:"type"`     // "thinking_delta", "text_delta", "signature_delta"
    Text     string `json:"text"`
    Thinking string `json:"thinking"`
}

// 콜백 인터페이스
type StreamCallbacks struct {
    OnText     func(text string)
    OnThinking func(text string)
    OnResult   func(result string)
}

func ParseStream(reader io.Reader, cb StreamCallbacks)
```

**버퍼링 전략:** 텍스트와 사고 과정 각각 별도 `strings.Builder`로 버퍼링하며, 줄바꿈 또는 80자 초과 시 콜백으로 flush합니다.

## 1.3 CLI 플래그 구성

```go
// agent.go & lead.go 공통
args := []string{"-p",
    "--output-format", "stream-json",
    "--include-partial-messages",  // ← v1.4 추가 (토큰 스트리밍 필수)
    "--verbose",
    "--dangerously-skip-permissions",
}
```

---

# 2. 사고 과정(Thinking) 표시

## 2.1 백엔드 → 프론트엔드 이벤트 흐름

```
Claude CLI stdout
    │
    ▼
ParseStream (streamparser.go)
    │
    ├─ OnThinking("분석 중...")  → logFn("💭 분석 중...")
    │                                    │
    ├─ OnText("응답 텍스트...")  → logFn("응답 텍스트...")
    │                                    │
    └─ OnResult("최종")         → fullResult = "최종"
                                         │
                                         ▼
                              app.go logFn 래퍼
                                         │
                              "💭" 포함 → LogEvent{type:"thinking"}
                              그 외     → LogEvent{type:"text"}
                                         │
                                         ▼
                              runtime.EventsEmit("log", LogEvent)
                                         │
                                         ▼
                              App.tsx EventsOn("log")
                                         │
                              pendingLogs 배치 큐에 추가
                                         │
                              requestAnimationFrame으로 flush
                                         │
                                         ▼
                              LogPanel (thinking 그룹 렌더링)
```

## 2.2 LogEvent 구조

```go
// app.go
type LogEvent struct {
    Type    string `json:"type"`    // "text", "thinking", "status"
    Message string `json:"message"`
}
```

```typescript
// App.tsx
interface LogEntry {
    type: 'text' | 'thinking' | 'status';
    message: string;
}
```

## 2.3 Thinking 그룹 렌더링

LogPanel에서 연속된 `thinking` 항목을 하나의 접을 수 있는(collapsible) 그룹으로 렌더링:

- 기본: 접혀있음 (`▶ 💭 사고 과정 (N개 청크)`)
- 클릭 시 펼침: 연한 배경 + 이탤릭으로 사고 내용 표시
- 최대 높이 200px, 오버플로우 스크롤

---

# 3. 프론트엔드 성능 최적화

## 3.1 문제

스트리밍 중 토큰마다 `setLogs(prev => [...prev, msg])` 호출 시:
- 매 토큰마다 배열 복사 O(n)
- React 리렌더링 초당 수백 회
- `scrollIntoView({ behavior: 'smooth' })` 애니메이션 큐 폭발

## 3.2 해결: requestAnimationFrame 배치

```typescript
// App.tsx
const pendingLogs = useRef<LogEntry[]>([]);
const flushTimer = useRef<number | null>(null);

const scheduleFlush = useCallback(() => {
    if (flushTimer.current !== null) return;
    flushTimer.current = requestAnimationFrame(() => {
        flushTimer.current = null;
        const batch = pendingLogs.current.splice(0);
        setLogs(prev => [...prev, ...batch]);
    });
}, []);
```

**효과:** 초당 수백 회 → 최대 60회(프레임 레이트) 리렌더링으로 감소.

## 3.3 스크롤 최적화

```typescript
// LogPanel.tsx
useEffect(() => {
    const container = containerRef.current;
    const isNearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 120;
    if (isNearBottom) {
        bottomRef.current?.scrollIntoView({ behavior: 'auto' }); // 'smooth' → 'auto'
    }
}, [logs]);
```

- `behavior: 'auto'` — 스트리밍 중 smooth 애니메이션 큐 방지
- 사용자가 위로 스크롤한 경우 자동 스크롤 비활성화
- `React.memo`로 불필요한 리렌더링 방지

---

# 4. 파일 변경 내역

## 4.1 신규 파일

| 파일 | 설명 |
|------|------|
| `gui/internal/streamparser.go` | 공유 Claude CLI 스트림 파서 |

## 4.2 수정된 파일

| 파일 | 변경 내용 |
|------|-----------|
| `gui/internal/agent.go` | 인라인 파싱 → ParseStream 사용, `--include-partial-messages` 추가, `bufio`/`encoding/json` import 제거 |
| `gui/internal/lead.go` | `streamEvent` 타입 삭제, `callClaudeStream` → ParseStream 사용, `--include-partial-messages` 추가, `bufio` import 제거 |
| `gui/app.go` | `LogEvent` 구조체 추가, 3곳의 logFn에서 text/thinking 구분 이벤트 발행 |
| `gui/frontend/src/App.tsx` | `LogEntry` 타입 도입, `requestAnimationFrame` 배치 업데이트, 구조화된 이벤트 수신 |
| `gui/frontend/src/components/LogPanel.tsx` | thinking 그룹 렌더링, 스크롤 최적화, `React.memo`, `LogEntry` export |

---

# 5. 이전 버전과의 호환성

- **프론트엔드 이벤트:** `EventsOn('log')` 핸들러가 string과 LogEvent 객체 모두 처리 (하위 호환)
- **Go 내부 API:** `LogFunc = func(string)` 시그니처 유지 — thinking은 `💭` 접두사로 구분
- **agent.Run() 시그니처:** `onStream ...func(string)` 유지 — 기존 호출부 변경 불필요

---

# 6. 개발 단계 로드맵 (업데이트)

```
Phase 1  Python MVP          오케스트레이션 핵심 로직 검증        ✅ 완료
Phase 2  권한 제어 강화       --allowedTools 도구 제한 추가        ✅ 완료
Phase 3  Go + Wails GUI       검증된 로직을 Go로 포팅 + GUI      ← 현재
  ├── Go 포팅 + React GUI                                        ✅ 완료
  ├── 세션 메모리 (session.json)                                  ✅ 완료
  ├── 동적 팀 구성 (PlanTeam)                                     ✅ 완료
  ├── Plan → Approve → Execute GUI 워크플로우                     ✅ 완료
  └── 실시간 스트리밍 + Thinking 표시                              ✅ v1.4
Phase 4  Docker 격리          컨테이너 기반 진짜 샌드박스
Phase 5  Docker Compose       전체 스택 통합 관리
```

---

# 7. 미결 사항 & 추후 논의

기존 v1.3 미결 사항에 추가:

- ~~`--output-format stream-json` 정식 지원 여부 확인~~ → ✅ 확인 완료, `--include-partial-messages` 필수
- 로그 가상화(virtualization) — 장시간 세션에서 수천 개 로그 항목 렌더링 성능 개선 (react-window 등)
- 에이전트별 로그 필터링 — 현재 모든 에이전트 로그가 단일 패널에 혼재
- Thinking 표시 개선 — 에이전트별 thinking 그룹 분리, 실시간 타이핑 애니메이션
- 스트리밍 중 에이전트 상태 실시간 업데이트 — 현재는 실행 완료 후 일괄 갱신
