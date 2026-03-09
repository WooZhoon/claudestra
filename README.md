# Claudestra

Claude Code 기반 멀티 에이전트 오케스트레이션 시스템.

여러 Claude Code 인스턴스가 팀을 이뤄 소프트웨어 개발 작업을 수행합니다.
팀장(Lead)이 요구사항을 분석하고, 팀원(Sub-Agent)에게 작업을 배분하고, 결과를 취합합니다.

---

## 설치 가이드

### 1단계: 사전 준비

아래 3가지가 설치되어 있어야 합니다.

#### Go (1.23 이상)

```bash
# Ubuntu / Debian
sudo apt update && sudo apt install -y golang-go

# macOS
brew install go

# 설치 확인
go version
```

#### Node.js (18 이상)

```bash
# Ubuntu / Debian (NodeSource)
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# macOS
brew install node

# 설치 확인
node --version
npm --version
```

#### Claude Code CLI

```bash
npm install -g @anthropic-ai/claude-code

# 설치 확인
claude --version
```

> Claude Code를 처음 사용하면 `claude` 명령어로 한 번 실행해서 API 키 설정을 완료하세요.

### 2단계: Wails 설치

Wails는 Go + 웹 기술로 데스크톱 앱을 만드는 프레임워크입니다.

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

시스템 의존성 확인:

```bash
wails doctor
```

> `wails doctor`에서 빨간 항목이 있으면 해당 패키지를 설치하세요.
> Linux의 경우 보통 `sudo apt install -y libgtk-3-dev libwebkit2gtk-4.0-dev` 가 필요합니다.

### 3단계: 프로젝트 클론 및 빌드

```bash
git clone https://github.com/WooZhoon/Claudestra.git
cd Claudestra/gui
```

#### CLI 도구 설치

```bash
go install ./cmd/claudestra
```

이 명령은 `claudestra` CLI 바이너리를 `~/go/bin/`에 설치합니다.
(`~/go/bin/`이 PATH에 없으면 추가하세요: `export PATH=$PATH:~/go/bin`)

#### GUI 실행 (개발 모드)

```bash
wails dev
```

처음 실행하면 npm 패키지 설치와 빌드에 시간이 걸립니다.
완료되면 Claudestra 창이 자동으로 열립니다.

#### GUI 빌드 (배포용)

```bash
wails build
```

빌드 결과물은 `build/bin/` 폴더에 생성됩니다.

---

## 사용법

### 1. 프로젝트 열기

GUI가 열리면 프로젝트 설정 화면이 나타납니다.

- **새 프로젝트**: 작업할 폴더를 선택하고 "새 프로젝트 생성" 클릭
- **기존 프로젝트 열기**: 이전에 사용한 프로젝트 폴더를 선택하고 "프로젝트 열기" 클릭

### 2. 요청 입력

하단 입력창에 원하는 작업을 한국어로 입력합니다.

```
예시:
- "틱택토 게임 만들어줘"
- "기존 코드에 로그인 기능 추가해줘"
- "코드 리팩토링 해줘"
```

전송 버튼을 누르면 팀장이 알아서:
1. 프로젝트 구조를 파악하고
2. 필요한 팀원(에이전트)을 구성하고
3. 각 팀원에게 작업을 배분하고 (병렬 가능)
4. 결과를 취합해서 보고서를 작성합니다

### 3. 실행 중 화면

- **중앙 로그 패널**: 팀장과 팀원들의 실시간 작업 내용
- **왼쪽 사이드바**: 팀원 목록과 상태 (대기/실행 중/완료)
- **팀원 클릭**: 오른쪽에 해당 팀원의 상세 정보 (지시, 실행 로그, 결과)
- **중지 버튼**: 실행 중일 때 빨간 "중지" 버튼으로 세션 강제 종료 가능

### 4. 권한 승인

에이전트가 파일 쓰기, 명령어 실행 등을 할 때 승인 다이얼로그가 뜹니다.
읽기 전용 명령어(ls, cat, git status 등)는 자동 허용됩니다.

### 5. 결과 확인

작업 완료 후 "보고서" 버튼을 눌러 최종 결과를 확인할 수 있습니다.

---

## 작동 원리

```
사용자 요청
    │
    ▼
┌──────────┐
│  팀장 AI  │  ← Claude Code 세션 1개
│  (Lead)   │
└────┬─────┘
     │  claudestra CLI로 팀 관리
     │
     ├── team set     → 팀원 구성
     ├── contract set → 인터페이스 계약서
     ├── assign       → 작업 지시 (동기/비동기)
     │
     ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│ 팀원 A   │  │ 팀원 B   │  │ 팀원 C   │  ← 각각 별도 Claude Code
│(Producer)│  │(Producer)│  │(Consumer)│
└──────────┘  └──────────┘  └──────────┘
     │              │              │
     └──────────────┴──────────────┘
                    │
                    ▼
              최종 보고서
```

- **Producer**: 코드를 직접 작성하는 팀원 (Read, Write, Edit, Bash 사용 가능)
- **Consumer**: 다른 팀원의 결과물을 분석하는 팀원 (Read 전용, 리뷰어/QA 등)

---

## 문제 해결

### `wails: command not found`

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

셸 설정 파일(`~/.bashrc` 또는 `~/.zshrc`)에 위 줄을 추가하세요.

### `claude: command not found`

```bash
npm install -g @anthropic-ai/claude-code
```

### Linux에서 GUI가 안 열릴 때

```bash
sudo apt install -y libgtk-3-dev libwebkit2gtk-4.0-dev
```

### API 키 관련 오류

Claude Code CLI에서 인증이 필요합니다. 터미널에서 `claude`를 한 번 실행해서 로그인하세요.

---

## 기술 스택

| 레이어 | 기술 |
|--------|------|
| GUI | Wails v2 (Go + WebView) |
| 프론트엔드 | React 18 + TypeScript + Vite |
| 백엔드 | Go 1.23 |
| AI 엔진 | Claude Code CLI (stream-json 모드) |
| 에이전트 격리 | subprocess + WorkDir + 파일 락 |
