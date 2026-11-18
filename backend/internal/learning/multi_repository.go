package learning

import (
	"context"
	"log/slog"
)

type MultiLearningRepository struct {
	primary   LearningRepository
	secondary LearningRepository
}

func NewMultiLearningRepository(primary, secondary LearningRepository) *MultiLearningRepository {
	return &MultiLearningRepository{primary: primary, secondary: secondary}
}

func (m *MultiLearningRepository) Create(ctx context.Context, log *LearningLog) error {
	if err := m.primary.Create(ctx, log); err != nil {
		return err
	}
	if err := m.secondary.Create(ctx, log); err != nil {
		slog.Warn("secondary learning storage write failed", "error", err)
	}
	return nil
}

func (m *MultiLearningRepository) FindAll(ctx context.Context) ([]LearningLog, error) {
	return m.primary.FindAll(ctx)
}

func (m *MultiLearningRepository) BatchCreate(ctx context.Context, logs []*LearningLog) error {
	if err := m.primary.BatchCreate(ctx, logs); err != nil {
		return err
	}
	if err := m.secondary.BatchCreate(ctx, logs); err != nil {
		slog.Warn("secondary learning batch write failed", "error", err)
	}
	return nil
}

func (m *MultiLearningRepository) BatchDelete(ctx context.Context, ids []int64) error {
	if err := m.primary.BatchDelete(ctx, ids); err != nil {
		return err
	}
	if err := m.secondary.BatchDelete(ctx, ids); err != nil {
		slog.Warn("secondary learning batch delete failed", "error", err)
	}
	return nil
}

// UpdateLog writes in.Values on both stores: primary first, then the
// secondary with the same values. Secondary failure is logged, not
// returned — the primary already committed the user's intended override
// and a missing DB row is recoverable via re-import.
func (m *MultiLearningRepository) UpdateLog(ctx context.Context, in UpdateLogInput) (UpdateLogResult, error) {
	res, err := m.primary.UpdateLog(ctx, in)
	if err != nil {
		return UpdateLogResult{}, err
	}
	if !res.Found {
		return res, nil
	}
	if _, err := m.secondary.UpdateLog(ctx, in); err != nil {
		slog.Warn("secondary learning override failed", "error", err)
	}
	return res, nil
}
