// Package workflowautomations stores and executes reviewed automation contracts.
package workflowautomations

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateName = errors.New("workflow automation name already exists")
	ErrInvalidInput  = errors.New("invalid workflow automation")
	ErrNotFound      = errors.New("workflow automation not found")
	ErrForbidden     = errors.New("workflow automation action forbidden")
	ErrNotExecutable = errors.New("workflow automation is not an executable task contract")
	ErrActiveLimit   = errors.New("workflow automation active task-action limit reached")
)

type Automation struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TriggerConfig    map[string]any `json:"triggerConfig"`
	ConditionLogic   string         `json:"conditionLogic"`
	Conditions       []Condition    `json:"conditions"`
	Actions          []Action       `json:"actions"`
	IsActive         bool           `json:"isActive"`
	Position         int            `json:"position"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Action struct {
	Type         string         `json:"type"`
	Config       map[string]any `json:"config"`
	DelayMinutes int            `json:"delayMinutes,omitempty"`
	ScheduledAt  *time.Time     `json:"scheduledAt,omitempty"`
}

type Run struct {
	ID               int64          `json:"id"`
	AutomationID     int64          `json:"automationId"`
	AutomationName   string         `json:"automationName"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TargetEntityID   int64          `json:"targetEntityId,omitempty"`
	TriggerEventKey  string         `json:"triggerEventKey"`
	Status           string         `json:"status"`
	TriggerPayload   map[string]any `json:"triggerPayload"`
	ConditionResult  *bool          `json:"conditionResult,omitempty"`
	ActionsTotal     int            `json:"actionsTotal"`
	ActionsCompleted int            `json:"actionsCompleted"`
	RetryCount       int            `json:"retryCount"`
	LastError        string         `json:"lastError"`
	ScheduledAt      string         `json:"scheduledAt"`
	StartedAt        string         `json:"startedAt"`
	CompletedAt      string         `json:"completedAt"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	Operation        *RunOperation  `json:"operation,omitempty"`
	Actions          []RunAction    `json:"actions"`
}

// RunOperation is the durable queue state for workflow outcomes that execute
// asynchronously. Keeping it attached to the run prevents a retryable or dead
// job from being presented as an indefinitely running automation.
type RunOperation struct {
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"maxAttempts"`
	LastError   string `json:"lastError"`
	RunAt       string `json:"runAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type RunListQuery struct {
	AutomationID int64
	Limit        int
}

type ListQuery struct {
	Page     int
	PageSize int
}

type ListPage struct {
	Automations       []Automation `json:"automations"`
	Page              int          `json:"page"`
	PageSize          int          `json:"pageSize"`
	Total             int          `json:"total"`
	ActiveActionCount int          `json:"activeActionCount"`
}

type RunInput struct {
	TriggerEventKey string         `json:"triggerEventKey"`
	TargetEntityID  int64          `json:"targetEntityId"`
	TriggerPayload  map[string]any `json:"triggerPayload"`
	ConditionResult *bool          `json:"conditionResult"`
	ActionsTotal    int            `json:"actionsTotal"`
	Status          string         `json:"status"`
}

type RunCompletionInput struct {
	Status           string `json:"status"`
	ConditionResult  *bool  `json:"conditionResult"`
	ActionsCompleted int    `json:"actionsCompleted"`
	RetryCount       int    `json:"retryCount"`
	LastError        string `json:"lastError"`
}

type Input struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	TriggerType      string         `json:"triggerType"`
	TargetEntityType string         `json:"targetEntityType"`
	TriggerConfig    map[string]any `json:"triggerConfig"`
	ConditionLogic   string         `json:"conditionLogic"`
	Conditions       []Condition    `json:"conditions"`
	Actions          []Action       `json:"actions"`
	IsActive         *bool          `json:"isActive"`
	Position         int            `json:"position"`
	DeactivateOnly   bool           `json:"deactivateOnly,omitempty"`
}

type Service struct {
	pool *pgxpool.Pool
}

const (
	DefaultDefinitionListPageSize = 50
	maxActionDelayMinutes         = 525600
	maxDefinitionNameLength       = 120
	maxDefinitionDescriptionLen   = 2000
	maxStoredDefinitionEntries    = 25
	maxDefinitionConditionValue   = 2000
	maxDefinitionPosition         = 1000000
)

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}
