# Claudestra

Claude Code 기반 멀티 에이전트 오케스트레이션 시스템

## 개요

Claudestra는 여러 Claude Code 인스턴스를 오케스트레이션하여 복잡한 소프트웨어 개발 작업을 자동화합니다.
팀장(Lead Agent)이 사용자 요구사항을 분석하고, 역할별 팀원(Sub-Agent)에게 태스크를 배분합니다.

## 주요 기능

- **Plan → Approve → Execute** 워크플로우
- **인터페이스 계약서** — 팀원 간 API/DB/네이밍 스펙 자동 통일
- **의존성 기반 태스크 그래프** — 토폴로지 정렬로 자동 병렬화
- **실시간 스트리밍** — 에이전트 응답을 토큰 단위로 실시간 표시
- **사고 과정(Thinking) 표시** — Extended Thinking을 접을 수 있는 그룹으로 표시
- **동적 팀 구성** — 요구사항에 맞는 팀원을 자동 생성
- **세션 메모리** — 이전 작업 컨텍스트를 기억하여 후속 요청 처리
- **역할별 도구 제한** — Producer (읽기/쓰기), Consumer (읽기 전용)
- **파일 락 레지스트리** — 에이전트 간 워크스페이스 충돌 방지

## 구성

```
Claudestra/
├── src/                  # Python MVP (CLI)
│   ├── main.py           # CLI 진입점
│   ├── lead_agent.py     # 팀장 AI
│   ├── agent.py          # 에이전트 기반 클래스
│   ├── workspace.py      # 워크스페이스 관리
│   └── file_lock.py      # 파일 락 레지스트리
├── gui/                  # Go + Wails GUI
│   ├── main.go           # Wails 앱 진입점
│   ├── app.go            # Wails 바인딩 (프론트엔드 ↔ 백엔드)
│   ├── internal/
│   │   ├── agent.go      # 에이전트 (스트리밍 실행)
│   │   ├── lead.go       # 팀장 AI (계획, 계약서, 세션 메모리)
│   │   ├── streamparser.go  # Claude CLI 스트림 파서
│   │   ├── workspace.go  # 워크스페이스 관리
│   │   └── filelock.go   # 파일 락 레지스트리
│   └── frontend/         # React 프론트엔드
│       └── src/
│           ├── App.tsx
│           └── components/
│               ├── LogPanel.tsx        # 실시간 로그 (thinking 그룹 지원)
│               ├── ProposalPanel.tsx   # 실행 계획 검토
│               ├── ReportPanel.tsx     # 최종 보고서
│               ├── Sidebar.tsx         # 에이전트 목록
│               ├── InputBar.tsx        # 사용자 입력
│               ├── AgentDetailPanel.tsx
│               └── ProjectSetup.tsx
├── docs/                 # 기술 설계 문서
└── Dockerfile
```

## 빠른 시작

### GUI (권장)

```bash
cd gui
wails dev
```

### Python CLI

```bash
cd src
pip install pyyaml

# 프로젝트 초기화
python main.py init --project ./my-project --roles backend,frontend,db,reviewer

# 실행
python main.py run --project ./my-project
```

## 에이전트 유형

| 유형 | 역할 예시 | 허용 도구 | 설명 |
|------|-----------|-----------|------|
| Producer | backend, frontend, db | Read, Write, Edit, Glob, Grep, Bash | 코드 작성 |
| Consumer | reviewer, doc_writer | Read, Glob, Grep | 읽기 전용 분석 |

## 기술 스택

| 레이어 | 기술 |
|--------|------|
| GUI 프레임워크 | Wails v2 (Go + WebView) |
| 프론트엔드 | React + TypeScript |
| AI 엔진 | Claude Code CLI (`stream-json` 모드) |
| 태스크 스케줄링 | 토폴로지 정렬 (Kahn's algorithm) |
| 에이전트 격리 | subprocess + WorkDir + 파일 락 |

## 설계 문서

- [v1.0 — 기본 아키텍처](docs/Orchestra_기술설계문서_v1.0.md)
- [v1.1 — 이데아 시스템](docs/Orchestra_기술설계문서_v1.1.md)
- [v1.2 — 계약서 시스템](docs/Claudestra_기술설계문서_v1.2.md)
- [v1.3 — 의존성 그래프, 메모리](docs/Claudestra_기술설계문서_v1.3.md)
- [v1.4 — 실시간 스트리밍, Thinking 표시](docs/Claudestra_기술설계문서_v1.4.md)
