package waitlist

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeStore struct {
	entries map[string]Entry
	err     error
}

func (f *fakeStore) Insert(_ context.Context, e Entry) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.entries == nil {
		f.entries = map[string]Entry{}
	}
	if _, ok := f.entries[e.EmailNormalized]; ok {
		return false, nil
	}
	f.entries[e.EmailNormalized] = e
	return true, nil
}

func TestJoinNormalisesAndDeduplicates(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	res, err := svc.Join(context.Background(), "  Ada@Studio.Example ", "studio")
	if err != nil || !res.Created {
		t.Fatalf("first join: res=%+v err=%v", res, err)
	}
	got := store.entries["ada@studio.example"]
	if got.Email != "Ada@Studio.Example" || got.Discipline != DisciplineStudio {
		t.Errorf("stored entry = %+v", got)
	}

	res, err = svc.Join(context.Background(), "ada@studio.example", "")
	if err != nil || res.Created {
		t.Fatalf("duplicate join: res=%+v err=%v", res, err)
	}
}

func TestJoinValidation(t *testing.T) {
	cases := []struct {
		name, email, discipline, field string
	}{
		{"empty email", "", "", "email"},
		{"no at", "ada.example.com", "", "email"},
		{"no dot in domain", "ada@localhost", "", "email"},
		{"display name", "Ada <ada@example.com>", "", "email"},
		{"trailing dot", "ada@example.", "", "email"},
		{"too long", "a@" + strings.Repeat("a", 260) + ".com", "", "email"},
		{"unknown discipline", "ada@example.com", "astronaut", "discipline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := NewService(store).Join(context.Background(), tc.email, tc.discipline)
			var verr *ValidationError
			if !errors.As(err, &verr) || verr.Field != tc.field {
				t.Fatalf("err = %v, want validation error on %s", err, tc.field)
			}
			if len(store.entries) != 0 {
				t.Error("invalid input was stored")
			}
		})
	}
}

func TestJoinAcceptsAllDisciplines(t *testing.T) {
	for _, d := range []string{"", "clipper", "creator", "musician", "studio", "agency", "brand", "other"} {
		if _, err := NewService(&fakeStore{}).Join(context.Background(), "a@b.co", d); err != nil {
			t.Errorf("discipline %q rejected: %v", d, err)
		}
	}
}

func TestJoinStoreFailure(t *testing.T) {
	_, err := NewService(&fakeStore{err: errors.New("db down")}).Join(context.Background(), "a@b.co", "")
	var verr *ValidationError
	if err == nil || errors.As(err, &verr) {
		t.Fatalf("err = %v, want non-validation error", err)
	}
}
