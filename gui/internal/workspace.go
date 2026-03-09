package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var ConsumerRoles = map[string]bool{
	"reviewer":   true,
	"doc_writer": true,
}

var DefaultIdeas = map[string]string{
	"backend": `당신은 백엔드 개발 전문가입니다.
담당 디렉토리: ./backend/

전문 영역:
- RESTful API 설계 및 구현
- 인증/인가 시스템 (JWT, OAuth2)
- 비즈니스 로직 구현
- 성능 최적화 및 캐싱 전략

작업 규칙:
- 자신의 디렉토리(./backend/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./backend/.agent-status 에 DONE을 기록합니다.
- 다른 팀원과의 인터페이스가 필요하면 주석으로 명확히 명세합니다.`,

	"frontend": `당신은 프론트엔드 개발 전문가입니다.
담당 디렉토리: ./frontend/

전문 영역:
- React / Vue 컴포넌트 설계
- 상태 관리 (Zustand, Redux)
- UI/UX 구현 및 반응형 디자인
- API 연동

작업 규칙:
- 자신의 디렉토리(./frontend/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./frontend/.agent-status 에 DONE을 기록합니다.
- 백엔드 API 명세가 필요하면 주석에 가정사항을 명확히 기록합니다.`,

	"db": `당신은 데이터베이스 전문가입니다.
담당 디렉토리: ./db/

전문 영역:
- 데이터베이스 스키마 설계
- SQL 쿼리 최적화
- 마이그레이션 스크립트 작성
- 인덱스 전략

작업 규칙:
- 자신의 디렉토리(./db/) 외부 파일은 절대 수정하지 않습니다.
- 작업 완료 시 ./db/.agent-status 에 DONE을 기록합니다.
- 모든 스키마 변경에는 롤백 스크립트를 함께 작성합니다.`,

	"reviewer": `당신은 코드 리뷰 전문가입니다.
담당 디렉토리: ./reviewer/

전문 영역:
- 코드 품질 분석
- 보안 취약점 탐지
- 성능 이슈 식별
- 코딩 컨벤션 검토

작업 규칙:
- 읽기 전용으로 다른 팀원의 코드를 참조합니다.
- 자신의 디렉토리(./reviewer/)에만 리뷰 결과를 작성합니다.
- 작업 완료 시 ./reviewer/.agent-status 에 DONE을 기록합니다.
- 비판보다 건설적인 개선 제안을 작성합니다.`,

	"doc_writer": `당신은 기술 문서 작성 전문가입니다.
담당 디렉토리: ./docs/

전문 영역:
- API 문서 작성 (OpenAPI/Swagger)
- README 및 가이드 문서
- 아키텍처 문서
- 코드 주석 정리

작업 규칙:
- 읽기 전용으로 다른 팀원의 코드를 참조합니다.
- 자신의 디렉토리(./docs/)에만 문서를 작성합니다.
- 작업 완료 시 ./docs/.agent-status 에 DONE을 기록합니다.
- 개발자와 비개발자 모두 이해할 수 있도록 문서를 작성합니다.`,
}

type WorkspaceConfig struct {
	Agents  []string `yaml:"agents"`
	Version string   `yaml:"version"`
}

type Workspace struct {
	Root         string
	OrchestraDir string
	IdeasDir     string
	LocksDir     string
	ContractsDir string
}

func NewWorkspace(root string) *Workspace {
	w := &Workspace{
		Root:         root,
		OrchestraDir: filepath.Join(root, ".orchestra"),
		IdeasDir:     filepath.Join(root, ".orchestra", "ideas"),
		LocksDir:     filepath.Join(root, ".orchestra", "locks"),
		ContractsDir: filepath.Join(root, ".orchestra", "contracts"),
	}
	os.MkdirAll(w.IdeasDir, 0755)
	os.MkdirAll(w.LocksDir, 0755)
	os.MkdirAll(w.ContractsDir, 0755)
	return w
}

func (w *Workspace) Init(roles []string) error {
	config := WorkspaceConfig{
		Agents:  roles,
		Version: "1.0",
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(filepath.Join(w.OrchestraDir, "config.yaml"), data, 0644)

	for _, role := range roles {
		agentDir := filepath.Join(w.Root, role)
		os.MkdirAll(agentDir, 0755)

		ideaFile := filepath.Join(w.IdeasDir, role+".yaml")
		if _, err := os.Stat(ideaFile); os.IsNotExist(err) {
			idea := DefaultIdeas[role]
			if idea == "" {
				idea = fmt.Sprintf("당신은 %s 전문가입니다.\n담당 디렉토리: ./%s/", role, role)
			}
			ideaData, _ := yaml.Marshal(map[string]string{"role": role, "idea": idea})
			os.WriteFile(ideaFile, ideaData, 0644)
		}
	}
	return nil
}

func (w *Workspace) LoadConfig() (*WorkspaceConfig, error) {
	data, err := os.ReadFile(filepath.Join(w.OrchestraDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	var config WorkspaceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (w *Workspace) LoadIdea(role string) string {
	ideaFile := filepath.Join(w.IdeasDir, role+".yaml")
	data, err := os.ReadFile(ideaFile)
	if err != nil {
		if idea, ok := DefaultIdeas[role]; ok {
			return idea
		}
		return fmt.Sprintf("당신은 %s 전문가입니다.", role)
	}
	var doc map[string]string
	yaml.Unmarshal(data, &doc)
	if idea, ok := doc["idea"]; ok {
		return idea
	}
	return fmt.Sprintf("당신은 %s 전문가입니다.", role)
}

func (w *Workspace) SaveIdea(role, idea string) error {
	ideaFile := filepath.Join(w.IdeasDir, role+".yaml")
	data, err := yaml.Marshal(map[string]string{"role": role, "idea": idea})
	if err != nil {
		return err
	}
	return os.WriteFile(ideaFile, data, 0644)
}

func (w *Workspace) GetProducerDirs(excludeRole string) []string {
	config, err := w.LoadConfig()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, role := range config.Agents {
		if !ConsumerRoles[role] && role != excludeRole {
			dirs = append(dirs, filepath.Join(w.Root, role))
		}
	}
	return dirs
}

func (w *Workspace) SaveContract(content string) error {
	return os.WriteFile(filepath.Join(w.ContractsDir, "contract.yaml"), []byte(content), 0644)
}

func (w *Workspace) LoadContract() string {
	data, err := os.ReadFile(filepath.Join(w.ContractsDir, "contract.yaml"))
	if err != nil {
		return ""
	}
	return string(data)
}

func (w *Workspace) IsConsumer(role string) bool {
	return ConsumerRoles[role]
}
