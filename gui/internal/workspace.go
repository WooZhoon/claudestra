package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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
	return &Workspace{
		Root:         root,
		OrchestraDir: filepath.Join(root, ".orchestra"),
		IdeasDir:     filepath.Join(root, ".orchestra", "ideas"),
		LocksDir:     filepath.Join(root, ".orchestra", "locks"),
		ContractsDir: filepath.Join(root, ".orchestra", "contracts"),
	}
}

func (w *Workspace) Init(roles []string) error {
	os.MkdirAll(w.IdeasDir, 0755)
	os.MkdirAll(w.LocksDir, 0755)
	os.MkdirAll(w.ContractsDir, 0755)

	config := WorkspaceConfig{
		Agents:  roles,
		Version: "2.0",
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	os.WriteFile(filepath.Join(w.OrchestraDir, "config.yaml"), data, 0644)

	for _, role := range roles {
		agentDir := filepath.Join(w.Root, role)
		os.MkdirAll(agentDir, 0755)
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

// ── RolePlan 저장/로드 ──

func (w *Workspace) SaveRolePlans(plans []RolePlan) error {
	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.OrchestraDir, "team.json"), data, 0644)
}

func (w *Workspace) LoadRolePlans() []RolePlan {
	data, err := os.ReadFile(filepath.Join(w.OrchestraDir, "team.json"))
	if err != nil {
		return nil
	}
	var plans []RolePlan
	if err := json.Unmarshal(data, &plans); err != nil {
		return nil
	}
	return plans
}

// ── 이데아 관리 ──

func (w *Workspace) LoadIdea(role string) string {
	ideaFile := filepath.Join(w.IdeasDir, role+".yaml")
	data, err := os.ReadFile(ideaFile)
	if err != nil {
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
	os.MkdirAll(w.IdeasDir, 0755)
	ideaFile := filepath.Join(w.IdeasDir, role+".yaml")
	data, err := yaml.Marshal(map[string]string{"role": role, "idea": idea})
	if err != nil {
		return err
	}
	return os.WriteFile(ideaFile, data, 0644)
}

// ── 계약서 ──

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
