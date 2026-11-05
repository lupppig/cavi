package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/lupppig/cavi/apps/api/internal/account"
	"github.com/lupppig/cavi/apps/api/internal/authz"
)

// Resolve returns the model an organisation uses for task: its own choice
// if it made one and the model is still supported, otherwise the platform
// default. It is for server-side use when work is created; it does no
// permission check.
func Resolve(ctx context.Context, db bun.IDB, orgID, task string) (Model, error) {
	var provider, id string
	err := db.QueryRowContext(ctx, `
		SELECT provider, model FROM organisation_ai_models WHERE organisation_id = ? AND task = ?`, orgID, task).
		Scan(&provider, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Model{}, fmt.Errorf("load ai model setting: %w", err)
	}
	if m, ok := Lookup(task, provider, id); ok {
		return m, nil
	}
	d := Default(task)
	if d.ID == "" {
		return Model{}, fmt.Errorf("ai: no default model for task %q", task)
	}
	return d, nil
}

// TaskSetting is an organisation's model choice for one task.
type TaskSetting struct {
	Task string
	// Model is the model in use; Custom is false when it's the default
	// because the organisation hasn't chosen.
	Model   Model
	Custom  bool
	Options []Model
}

// Settings lists the organisation's model for every task. Any member may
// read them.
func Settings(ctx context.Context, db bun.IDB, actorID, orgID string) ([]TaskSetting, error) {
	if _, err := account.ResolveAccess(ctx, db, actorID, orgID, false); err != nil {
		return nil, err
	}
	out := make([]TaskSetting, 0, len(Tasks()))
	for _, task := range Tasks() {
		m, err := Resolve(ctx, db, orgID, task)
		if err != nil {
			return nil, err
		}
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM organisation_ai_models WHERE organisation_id = ? AND task = ? AND provider = ? AND model = ?)`,
			orgID, task, m.Provider, m.ID).Scan(&exists); err != nil {
			return nil, err
		}
		out = append(out, TaskSetting{Task: task, Model: m, Custom: exists, Options: Models(task)})
	}
	return out, nil
}

// SetModel chooses the model an organisation uses for task
// (organisation.manage). An empty model ID returns to the platform default.
// Only models in the catalogue are accepted.
func SetModel(ctx context.Context, db *bun.DB, actorID, orgID, task, provider, modelID string) error {
	if Default(task).ID == "" {
		return &account.ValidationError{Field: "task", Message: "Choose a supported AI task."}
	}
	if modelID != "" {
		if _, ok := Lookup(task, provider, modelID); !ok {
			return &account.ValidationError{Field: "model", Message: "Choose one of the supported models."}
		}
	}
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		a, err := account.ResolveAccess(ctx, tx, actorID, orgID, false)
		if err != nil {
			return err
		}
		if !authz.Can(a.Role, authz.OrganisationManage) {
			return account.ErrForbidden
		}
		if modelID == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM organisation_ai_models WHERE organisation_id = ? AND task = ?`, orgID, task); err != nil {
				return fmt.Errorf("reset ai model: %w", err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			INSERT INTO organisation_ai_models (organisation_id, task, provider, model, updated_by_user_id)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (organisation_id, task) DO UPDATE SET
				provider = EXCLUDED.provider, model = EXCLUDED.model,
				updated_by_user_id = EXCLUDED.updated_by_user_id, updated_at = now()`,
			orgID, task, provider, modelID, actorID); err != nil {
			return fmt.Errorf("set ai model: %w", err)
		}
		return account.Audit(ctx, tx, orgID, actorID, "ai.model_changed", "organisation", orgID,
			map[string]any{"task": task, "provider": provider, "model": modelID})
	})
}
