# 🎼 Orchestra MVP

Claude Code 기반 멀티 에이전트 오케스트레이션 시스템 — Python MVP

## 설치

```bash
pip install -r requirements.txt
```

> Claude Code가 설치되어 있어야 합니다: https://docs.anthropic.com/claude-code

## 사용법

### 1. 프로젝트 초기화

```bash
python main.py init --project ./my-project --roles backend,frontend,db,reviewer
```

### 2. 오케스트레이션 실행

```bash
python main.py run --project ./my-project
```

실행 후 요청을 입력하면 팀장 AI가 분석해서 팀원들에게 배분합니다.

```
📝 요청 입력 > 사용자 인증 API와 로그인 페이지를 만들어줘
```

### 3. 이데아 편집

```bash
python main.py idea --project ./my-project
```

### 4. 상태 확인

```bash
python main.py status --project ./my-project
```

## 디렉토리 구조

```
my-project/
├── .orchestra/
│   ├── config.yaml       ← 프로젝트 설정
│   └── ideas/            ← 에이전트별 이데아 (편집 가능)
│       ├── backend.yaml
│       ├── frontend.yaml
│       └── db.yaml
├── backend/              ← Backend 에이전트 작업 공간
├── frontend/             ← Frontend 에이전트 작업 공간
├── db/                   ← DB 에이전트 작업 공간
└── reviewer/             ← Reviewer 에이전트 작업 공간 (읽기 전용 참조)
```

## 에이전트 유형

| 유형 | 역할 예시 | 동작 |
|------|-----------|------|
| Producer | backend, frontend, db | 자신의 디렉토리에서 작업 |
| Consumer | reviewer, doc_writer | 다른 디렉토리를 읽기 전용 참조 |

## 파일 구성

```
orchestra/
├── main.py         ← 진입점 (CLI)
├── lead_agent.py   ← 팀장 AI (태스크 분해 & 배분)
├── agent.py        ← 에이전트 기반 클래스 (Claude Code subprocess 래핑)
├── workspace.py    ← 워크스페이스 & 이데아 관리
└── requirements.txt
```
