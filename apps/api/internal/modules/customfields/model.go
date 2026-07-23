package customfields

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrConflict      = errors.New("custom field conflict")
	ErrInvalidInput  = errors.New("invalid custom field input")
	ErrNotFound      = errors.New("custom field not found")
	ErrInactiveActor = errors.New("custom field actor is not an active organization member")
	ErrForbidden     = errors.New("custom field actor is not an organization administrator")
	ErrChanged       = errors.New("custom field changed")
)

const MaxDefinitionsPerEntity = 25

type Values map[string]json.RawMessage

type Definition struct {
	ID              int64      `json:"id"`
	EntityType      string     `json:"entityType"`
	FieldKey        string     `json:"fieldKey"`
	Label           string     `json:"label"`
	DataType        string     `json:"dataType"`
	Options         []string   `json:"options"`
	Required        bool       `json:"required"`
	ShowInList      bool       `json:"showInList"`
	Position        int        `json:"position"`
	Revision        int        `json:"revision"`
	CreatedByUserID int64      `json:"createdByUserId"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	ArchivedAt      *time.Time `json:"archivedAt,omitempty"`
}

type CreateInput struct {
	EntityType string   `json:"entityType"`
	FieldKey   string   `json:"fieldKey"`
	Label      string   `json:"label"`
	DataType   string   `json:"dataType"`
	Options    []string `json:"options"`
	Required   bool     `json:"required"`
	ShowInList bool     `json:"showInList"`
	Position   int      `json:"position"`
}

type UpdateInput struct {
	Label      string   `json:"label"`
	Options    []string `json:"options"`
	Required   bool     `json:"required"`
	ShowInList bool     `json:"showInList"`
	Position   int      `json:"position"`
	Revision   int      `json:"revision"`
}

type Filter struct {
	FieldKey string `json:"fieldKey,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
}

type NormalizedFilter struct {
	Definition Definition
	Operator   string
	Value      string
	Boolean    bool
}
