// internal/client/store/tags_test.go
package store_test

import (
	"testing"

	"github.com/jaypaulb/trasker/internal/client/store"
)

func TestCreateTag(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateTag("Development", "#3B82F6")
	if err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	tag, err := s.GetTag(id)
	if err != nil {
		t.Fatalf("GetTag() error: %v", err)
	}
	if tag.Name != "Development" {
		t.Errorf("Name = %q, want %q", tag.Name, "Development")
	}
	if tag.Color != "#3B82F6" {
		t.Errorf("Color = %q, want %q", tag.Color, "#3B82F6")
	}
}

func TestCreateTag_DuplicateName(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateTag("Dev", "#000000")
	if err != nil {
		t.Fatalf("first CreateTag() error: %v", err)
	}

	_, err = s.CreateTag("Dev", "#FFFFFF")
	if err == nil {
		t.Fatal("expected error for duplicate tag name, got nil")
	}
}

func TestListTags(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.CreateTag("Alpha", "#111111")
	_, _ = s.CreateTag("Beta", "#222222")
	_, _ = s.CreateTag("Gamma", "#333333")

	tags, err := s.ListTags()
	if err != nil {
		t.Fatalf("ListTags() error: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("got %d tags, want 3", len(tags))
	}
}

func TestUpdateTag(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateTag("OldName", "#000000")
	err := s.UpdateTag(id, "NewName", "#FFFFFF")
	if err != nil {
		t.Fatalf("UpdateTag() error: %v", err)
	}

	tag, _ := s.GetTag(id)
	if tag.Name != "NewName" {
		t.Errorf("Name = %q, want %q", tag.Name, "NewName")
	}
	if tag.Color != "#FFFFFF" {
		t.Errorf("Color = %q, want %q", tag.Color, "#FFFFFF")
	}
}

func TestDeleteTag(t *testing.T) {
	s := newTestStore(t)

	id, _ := s.CreateTag("Doomed", "#000000")
	err := s.DeleteTag(id)
	if err != nil {
		t.Fatalf("DeleteTag() error: %v", err)
	}

	_, err = s.GetTag(id)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

// Verify the Tag struct is exported properly.
func TestTag_Fields(t *testing.T) {
	tag := store.Tag{
		ID:    1,
		Name:  "Test",
		Color: "#AABBCC",
	}
	if tag.ID != 1 || tag.Name != "Test" || tag.Color != "#AABBCC" {
		t.Error("Tag struct fields not working as expected")
	}
}
