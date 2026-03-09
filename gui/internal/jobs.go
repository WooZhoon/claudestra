package internal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Job struct {
	ID          string `json:"id"`
	Agent       string `json:"agent"`
	Status      string `json:"status"` // "running", "done", "error"
	Instruction string `json:"instruction"`
	PID         int    `json:"pid"`
	Output      string `json:"output,omitempty"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at,omitempty"`
}

func NewJobID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func JobPath(jobsDir, jobID string) string {
	return filepath.Join(jobsDir, "job-"+jobID+".json")
}

func SaveJob(jobsDir string, job *Job) error {
	os.MkdirAll(jobsDir, 0755)
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(JobPath(jobsDir, job.ID), data, 0644)
}

func LoadJob(jobsDir, jobID string) (*Job, error) {
	data, err := os.ReadFile(JobPath(jobsDir, jobID))
	if err != nil {
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func ListJobs(jobsDir string) []*Job {
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		return nil
	}
	var jobs []*Job
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "job-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(jobsDir, e.Name()))
		if err != nil {
			continue
		}
		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		jobs = append(jobs, &job)
	}
	return jobs
}

func CreateRunningJob(jobsDir, agent, instruction string, pid int) *Job {
	job := &Job{
		ID:          NewJobID(),
		Agent:       agent,
		Status:      "running",
		Instruction: instruction,
		PID:         pid,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	SaveJob(jobsDir, job)
	return job
}

func FinishJob(jobsDir string, job *Job, status, output string) {
	job.Status = status
	job.Output = output
	job.FinishedAt = time.Now().Format(time.RFC3339)
	SaveJob(jobsDir, job)
}
