package kb

import (
	"context"

	"github.com/graffvishenka/polar-star-service/backend/internal/files"
)

// MediaValidator — доступ к медиа статей (KB-5). Сервис БЗ не работает с files
// напрямую: интерфейс позволяет подменить реализацию в тестах.
type MediaValidator interface {
	// ValidateMedia проверяет, что файл существует и имеет scope=kb, иначе
	// ErrMediaNotFound (KB-5: 400 INVALID_MEDIA_REF).
	ValidateMedia(ctx context.Context, fileID int64) error
	// MarkLinked помечает файлы использованными (files.linked_at), чтобы
	// фоновая очистка Б3 не удалила их как orphan.
	MarkLinked(ctx context.Context, fileIDs []int64) error
}

// FilesMediaValidator — реализация MediaValidator поверх files.Service (Б3).
type FilesMediaValidator struct {
	svc *files.Service
}

// NewFilesMediaValidator связывает домен БЗ с сервисом файлов.
func NewFilesMediaValidator(svc *files.Service) *FilesMediaValidator {
	return &FilesMediaValidator{svc: svc}
}

// ValidateMedia проверяет существование файла и область scope=kb. Файл заявки
// (scope=ticket) в статью вставить нельзя — это утечка вложений заявок в
// публичную базу знаний.
func (v *FilesMediaValidator) ValidateMedia(ctx context.Context, fileID int64) error {
	if v == nil || v.svc == nil || fileID <= 0 {
		return ErrMediaNotFound
	}
	file, err := v.svc.Get(ctx, fileID)
	if err != nil {
		return ErrMediaNotFound
	}
	if file.Scope != files.ScopeKB {
		return ErrMediaNotFound
	}
	return nil
}

// MarkLinked помечает медиа статьи использованными.
func (v *FilesMediaValidator) MarkLinked(ctx context.Context, fileIDs []int64) error {
	if v == nil || v.svc == nil || len(fileIDs) == 0 {
		return nil
	}
	if err := v.svc.MarkLinked(ctx, fileIDs); err != nil {
		return err
	}
	return nil
}
