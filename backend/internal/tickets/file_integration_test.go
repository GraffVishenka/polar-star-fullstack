package tickets

import (
	"context"
	"errors"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// fakeFileStore — FileAccessStore для проверки авторизаторов без MySQL.
type fakeFileStore struct {
	ticketByTokenHash map[string]int64
	fileToTicket      map[int64]int64
	ticketFiles       map[int64]map[int64]bool
}

func (s *fakeFileStore) TicketIDByAccessTokenHash(_ context.Context, hash string) (int64, error) {
	if id, ok := s.ticketByTokenHash[hash]; ok {
		return id, nil
	}
	return 0, ErrAccessTokenNotFound
}

func (s *fakeFileStore) FileBelongsToTicket(_ context.Context, fileID, ticketID int64) (bool, error) {
	if set, ok := s.ticketFiles[ticketID]; ok {
		return set[fileID], nil
	}
	return false, nil
}

func (s *fakeFileStore) TicketIDByFileID(_ context.Context, fileID int64) (int64, error) {
	if id, ok := s.fileToTicket[fileID]; ok {
		return id, nil
	}
	return 0, ErrFileNotAttached
}

func TestTicketTokenAuthorizer(t *testing.T) {
	store := &fakeFileStore{
		ticketByTokenHash: map[string]int64{auth.HashToken("secret-token"): 42},
		ticketFiles:       map[int64]map[int64]bool{42: {7: true}},
	}
	authorizer := NewTicketTokenAuthorizer(store)
	ctx := context.Background()

	if id, err := authorizer.TicketIDByToken(ctx, "secret-token"); err != nil || id != 42 {
		t.Fatalf("TicketIDByToken = %d, %v", id, err)
	}
	if _, err := authorizer.TicketIDByToken(ctx, "wrong"); !errors.Is(err, files.ErrTokenNotFound) {
		t.Fatalf("чужой токен: %v", err)
	}
	if ok, err := authorizer.FileBelongsToTicketToken(ctx, 7, "secret-token"); err != nil || !ok {
		t.Fatalf("свой файл: ok=%v err=%v", ok, err)
	}
	if ok, err := authorizer.FileBelongsToTicketToken(ctx, 8, "secret-token"); err != nil || ok {
		t.Fatalf("чужой файл: ok=%v err=%v", ok, err)
	}
	if ok, err := authorizer.FileBelongsToTicketToken(ctx, 7, "wrong"); err != nil || ok {
		t.Fatalf("чужой токен: ok=%v err=%v", ok, err)
	}
}

func TestFileAccessAuthorizer(t *testing.T) {
	_, fixture := newTestHandler(t)
	ticket := fixture.repo.addTicket(&Ticket{BuildingID: 1, ApplicantName: "A", Cabinet: "1", ProblemTypeID: 10})
	otherBuilding := int64Ptr(2)
	uploader := int64Ptr(5)

	store := &fakeFileStore{
		fileToTicket: map[int64]int64{100: ticket.ID},
	}
	authorizer := NewFileAccessAuthorizer(fixture.repo, store)
	ctx := context.Background()

	admin := fixture.userActor(1)
	userOfBuilding := fixture.userActor(5)
	foreign := &auth.User{ID: 77, Login: "other", Role: auth.RoleUser, IsActive: true, BuildingID: otherBuilding}

	attached := &files.File{ID: 100, Scope: files.ScopeTicket}
	if ok, err := authorizer.CanReadFile(ctx, userOfBuilding, attached); err != nil || !ok {
		t.Fatalf("USER своего здания: ok=%v err=%v", ok, err)
	}
	if ok, err := authorizer.CanReadFile(ctx, foreign, attached); err != nil || ok {
		t.Fatalf("USER чужого здания: ok=%v err=%v", ok, err)
	}
	if ok, err := authorizer.CanReadFile(ctx, admin, attached); err != nil || !ok {
		t.Fatalf("ADMIN: ok=%v err=%v", ok, err)
	}

	kb := &files.File{ID: 200, Scope: files.ScopeKB}
	if ok, err := authorizer.CanReadFile(ctx, foreign, kb); err != nil || !ok {
		t.Fatalf("KB-медиа: ok=%v err=%v", ok, err)
	}

	unattached := &files.File{ID: 300, Scope: files.ScopeTicket, UploaderID: uploader}
	if ok, err := authorizer.CanReadFile(ctx, userOfBuilding, unattached); err != nil || !ok {
		t.Fatalf("свой непривязанный файл: ok=%v err=%v", ok, err)
	}
	if ok, err := authorizer.CanReadFile(ctx, foreign, unattached); err != nil || ok {
		t.Fatalf("чужой непривязанный файл: ok=%v err=%v", ok, err)
	}
}

func TestFilesAttacher_ValidateScope(t *testing.T) {
	attacher := NewFilesAttacher(nil)
	ctx := context.Background()

	if err := attacher.ValidateScope(ctx, []int64{1, 2}, "ticket"); err != nil {
		t.Fatalf("валидные вложения: %v", err)
	}
	var apiErr *apierr.Error
	if err := attacher.ValidateScope(ctx, []int64{1, 2, 3, 4, 5, 6}, "ticket"); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("больше 5 вложений: %v", err)
	}
	if err := attacher.ValidateScope(ctx, []int64{1}, "kb"); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("чужая область: %v", err)
	}
}
