package models

import "time"

type Scope string

const (
	ScopeGlobal    Scope = "global"
	ScopeProject   Scope = "project"
	ScopeWorkspace Scope = "workspace"
)

type Category string

const (
    CategoryDecision   Category = "decision"
    CategoryBug        Category = "bug"
    CategoryPattern    Category = "pattern"
    CategoryNote       Category = "note"
    CategoryRequest    Category = "request"
    CategoryPreference Category = "preference"
)

type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionCompacted SessionStatus = "compacted"
	SessionAbandoned SessionStatus = "abandoned"
)

type RequestStatus string

const (
	RequestPending    RequestStatus = "pending"
	RequestInProgress RequestStatus = "in_progress"
	RequestCompleted  RequestStatus = "completed"
	RequestDeferred   RequestStatus = "deferred"
)

type RequestPriority string

const (
	PriorityLow      RequestPriority = "low"
	PriorityNormal   RequestPriority = "normal"
	PriorityHigh     RequestPriority = "high"
	PriorityCritical RequestPriority = "critical"
)

type RelationType string

const (
	RelCausedBy    RelationType = "caused_by"
	RelSupersedes  RelationType = "supersedes"
	RelRelatesTo   RelationType = "relates_to"
	RelContradicts RelationType = "contradicts"
	RelDependsOn   RelationType = "depends_on"
	RelDerivedFrom RelationType = "derived_from"
)

// ------------------------------
// Struct principal
// ------------------------------

type Project struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Path *string `json:"path,omitempty"`
	GitRemote *string `json:"git_remote,omitempty"`
	Workspace *string `json:"workpace,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Session struct {
	ID string `json:"id"`
	ProjectID *string `json:"project_id,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt *time.Time `json:"ended_at,omitempty"`
	Status SessionStatus `json:"status"`
	Objectives []string `json:"objectives"`
	Summary *string `json:"summary,omitempty"`
	TasksCompleted []string `json:"tasks_completed"`
	FilesTouched []string `json:"files_touched"`
	CompactionCount int `json:"compaction_count"`
}

type Observation struct {
	ID string `json:"id"`
	SessionID string `json:"session_id"`
	ProjectID *string `json:"project_id,omitempty"`
	Scope Scope `json:"scope"`
	Category Category `json:"category"`
	Title string `json:"title"`
	Content string `json:"content"`
	Tags []string `json:"tags"`
	Files []string `json:"files"`
	RelevanceScore float64 `json:"relevance_score"`
	AccessCount int `json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	SupersededBy *int `json:"superseded_by,omitempty"`
	IsActive bool `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Relation struct {
	ID           int          `json:"id"`
	SourceID     int          `json:"source_id"`
	TargetID     int          `json:"target_id"`
	RelationType RelationType `json:"relation_type"`
	Description  *string      `json:"description,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type UserRequest struct {
	ID          int             `json:"id"`
	SessionID   string          `json:"session_id"`
	ProjectID   *string         `json:"project_id,omitempty"`
	Request     string          `json:"request"`
	Priority    RequestPriority `json:"priority"`
	Status      RequestStatus   `json:"status"`
	Resolution  *string         `json:"resolution,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

type CompactionSnapshot struct {
	ID                    int       `json:"id"`
	SessionID             string    `json:"session_id"`
	SnapshotType          string    `json:"snapshot_type"`
	Summary               string    `json:"summary"`
	RawLength             *int      `json:"raw_length,omitempty"`
	ObservationsExtracted int       `json:"observations_extracted"`
	CreatedAt             time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────
// Inputs (lo que reciben los bit_* tools)
// ─────────────────────────────────────────────

type SaveObservationInput struct {
	SessionID string   `json:"session_id"`
	ProjectID *string  `json:"project_id,omitempty"`
	Scope     Scope    `json:"scope"`
	Category  Category `json:"category"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags,omitempty"`
	Files     []string `json:"files,omitempty"`
}

type SearchInput struct {
	Query    string    `json:"query"`
	Category *Category `json:"category,omitempty"`
	Project  *string   `json:"project,omitempty"`
	Scope    *Scope    `json:"scope,omitempty"`
	Limit    int       `json:"limit"`
}

type ContextInput struct {
	Project         *string `json:"project,omitempty"`
	Workspace       *string `json:"workspace,omitempty"`
	MaxTokens       int     `json:"max_tokens"`
	IncludeRequests bool    `json:"include_requests"`
}

// ─────────────────────────────────────────────
// Outputs (lo que devuelven los bit_* tools)
// ─────────────────────────────────────────────

type SearchResult struct {
	Observation    Observation `json:"observation"`
	EffectiveScore float64    `json:"effective_score"`
}

type ContextResponse struct {
	RecentSessions  []Session     `json:"recent_sessions"`
	Decisions       []Observation `json:"decisions"`
	Bugs            []Observation `json:"bugs"`
	Patterns        []Observation `json:"patterns"`
	Notes           []Observation `json:"notes"`
	PendingRequests []UserRequest `json:"pending_requests"`
	TotalItems      int           `json:"total_items"`
}
	
	
	
