package session

import "time"

type Session struct {
	ID          string    `json:"id"`
	ProjectRoot string    `json:"project_root"`
	Title       string    `json:"title"`
	AgentMD     string    `json:"agent_md,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store interface {
	Create(projectRoot, title, agentMD string) (*Session, error)
	GetActive(projectRoot string) (*Session, error)
	List(projectRoot string) ([]Session, error)
	Switch(projectRoot, sessionID string) (*Session, error)
	Delete(projectRoot, sessionID string) error
	Close()
}
