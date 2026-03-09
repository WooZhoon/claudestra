package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type AgentStatus string

const (
	StatusIdle    AgentStatus = "IDLE"
	StatusRunning AgentStatus = "RUNNING"
	StatusDone    AgentStatus = "DONE"
	StatusError   AgentStatus = "ERROR"
)

// Producer (코드 작성자): 파일 읽기/쓰기/검색 + Bash
var ProducerTools = []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"}

// Consumer (리뷰어 등): 읽기 전용
var ConsumerTools = []string{"Read", "Glob", "Grep"}

type AgentConfig struct {
	AgentID      string
	Role         string
	Idea         string
	WorkDir      string
	ReadRefs     []string // Consumer 전용 읽기 전용 참조 경로
	Contract     string   // 인터페이스 계약서
	AllowedTools []string // 허용 도구 목록
	IsConsumer   bool     // consumer 유형 여부
}

type Agent struct {
	Config          AgentConfig
	LockRegistry    *FileLockRegistry
	Status          AgentStatus
	Output          string
	LastInstruction string
	mu              sync.Mutex
}

func NewAgent(config AgentConfig, lockRegistry *FileLockRegistry) *Agent {
	os.MkdirAll(config.WorkDir, 0755)
	a := &Agent{
		Config:       config,
		LockRegistry: lockRegistry,
		Status:       StatusIdle,
	}
	a.writeStatus(StatusIdle)
	return a
}

func (a *Agent) Run(instruction string, onStream ...func(string)) string {
	a.mu.Lock()
	a.Status = StatusRunning
	a.LastInstruction = instruction
	a.writeStatus(StatusRunning)
	a.mu.Unlock()

	var streamFn func(string)
	if len(onStream) > 0 {
		streamFn = onStream[0]
	}

	log := func(msg string) {
		if streamFn != nil {
			streamFn(msg)
		}
	}

	// 잠금 획득
	if a.LockRegistry != nil {
		if err := a.LockRegistry.Acquire(a.Config.WorkDir, a.Config.AgentID); err != nil {
			a.Status = StatusError
			a.writeStatus(StatusError)
			a.Output = fmt.Sprintf("LOCK CONFLICT: %s", err)
			log(fmt.Sprintf("[%s] 🔒 잠금 충돌: %s", a.Config.Role, err))
			return a.Output
		}
		log(fmt.Sprintf("[%s] 🔒 잠금 획득: %s/", a.Config.Role, filepath.Base(a.Config.WorkDir)))
	}

	defer func() {
		if a.LockRegistry != nil {
			a.LockRegistry.ReleaseAll(a.Config.AgentID)
			log(fmt.Sprintf("[%s] 🔓 잠금 해제", a.Config.Role))
		}
	}()

	prompt := a.buildPrompt(instruction)
	truncated := instruction
	if len(truncated) > 60 {
		truncated = truncated[:60]
	}
	log(fmt.Sprintf("[%s] 🚀 시작: %s...", a.Config.Role, truncated))

	// claude 명령 구성 (stream-json 모드)
	args := []string{"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--dangerously-skip-permissions",
	}
	if len(a.Config.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(a.Config.AllowedTools, ","))
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = a.Config.WorkDir
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.Status = StatusError
		a.writeStatus(StatusError)
		a.Output = fmt.Sprintf("PIPE ERROR: %s", err)
		return a.Output
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		a.Status = StatusError
		a.writeStatus(StatusError)
		a.Output = fmt.Sprintf("START ERROR: %s", err)
		return a.Output
	}

	done := make(chan struct{})
	var fullResult string

	go func() {
		defer close(done)
		ParseStream(stdout, StreamCallbacks{
			OnText: func(text string) {
				log(fmt.Sprintf("[%s] %s", a.Config.Role, text))
			},
			OnThinking: func(text string) {
				log(fmt.Sprintf("[%s] 💭 %s", a.Config.Role, text))
			},
			OnResult: func(result string) {
				fullResult = result
			},
		})
	}()

	// 5분 타임아웃
	select {
	case <-done:
		cmd.Wait()
		a.Output = strings.TrimSpace(fullResult)
		a.Status = StatusDone
		a.writeStatus(StatusDone)
		log(fmt.Sprintf("[%s] ✅ 완료", a.Config.Role))
	case <-time.After(5 * time.Minute):
		cmd.Process.Kill()
		a.Output = "TIMEOUT: 5분 초과"
		a.Status = StatusError
		a.writeStatus(StatusError)
		log(fmt.Sprintf("[%s] ⏰ 타임아웃", a.Config.Role))
	}

	return a.Output
}

func (a *Agent) RunAsync(instruction string, onStream ...func(string)) chan string {
	ch := make(chan string, 1)
	go func() {
		result := a.Run(instruction, onStream...)
		ch <- result
	}()
	return ch
}

func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Status = StatusIdle
	a.Output = ""
	a.writeStatus(StatusIdle)
}

func (a *Agent) buildPrompt(instruction string) string {
	var parts []string

	parts = append(parts, a.Config.Idea)

	parts = append(parts, `[작업 원칙]
- 간결하게 핵심만 작성하세요. 전체 코드를 모두 작성하지 마세요.
- 핵심 구조, 주요 파일, 중요 로직만 구현하세요.
- 보일러플레이트나 반복적인 코드는 생략하고 주석으로 표시하세요.`)

	if a.Config.Contract != "" {
		parts = append(parts, fmt.Sprintf("[인터페이스 계약서 — 반드시 이 명세를 따르세요]\n%s", a.Config.Contract))
	}

	if len(a.Config.ReadRefs) > 0 {
		refs := strings.Join(a.Config.ReadRefs, "\n  - ")
		parts = append(parts, fmt.Sprintf("[읽기 전용 참조 디렉토리 — 수정 금지]\n  - %s", refs))
	}

	parts = append(parts, fmt.Sprintf("[지시]\n%s", instruction))

	return strings.Join(parts, "\n\n")
}

func (a *Agent) writeStatus(status AgentStatus) {
	statusFile := filepath.Join(a.Config.WorkDir, ".agent-status")
	os.WriteFile(statusFile, []byte(string(status)), 0644)
}
