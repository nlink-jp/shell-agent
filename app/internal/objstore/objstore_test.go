package objstore

import (
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello world")
	id, err := s.Save(data, TypeBlob, "text/plain", "test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	loaded, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != "hello world" {
		t.Errorf("got %q, want %q", string(loaded), "hello world")
	}
}

func TestSaveDataURL(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Minimal valid PNG (1x1 transparent pixel)
	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	id, err := s.SaveDataURL(dataURL, TypeImage, "test.png")
	if err != nil {
		t.Fatal(err)
	}

	meta, ok := s.Get(id)
	if !ok {
		t.Fatal("expected meta")
	}
	if meta.MimeType != "image/png" {
		t.Errorf("got mime %q, want image/png", meta.MimeType)
	}
	if meta.Type != TypeImage {
		t.Errorf("got type %q, want image", meta.Type)
	}

	du, err := s.LoadAsDataURL(id)
	if err != nil {
		t.Fatal(err)
	}
	if du[:15] != "data:image/png;" {
		t.Errorf("unexpected data URL prefix: %q", du[:15])
	}
}

func TestList(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	s.Save([]byte("img1"), TypeImage, "image/png", "a.png")
	s.Save([]byte("img2"), TypeImage, "image/png", "b.png")
	s.Save([]byte("blob1"), TypeBlob, "text/plain", "c.txt")

	images := s.List(TypeImage)
	if len(images) != 2 {
		t.Errorf("expected 2 images, got %d", len(images))
	}
	blobs := s.List(TypeBlob)
	if len(blobs) != 1 {
		t.Errorf("expected 1 blob, got %d", len(blobs))
	}
}

func TestDelete(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	id, _ := s.Save([]byte("data"), TypeBlob, "text/plain", "test.txt")
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}

	_, err = s.Load(id)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	s1, _ := New(dir)
	id, _ := s1.Save([]byte("persistent"), TypeBlob, "text/plain", "test.txt")

	// Reopen
	s2, _ := New(dir)
	data, err := s2.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "persistent" {
		t.Errorf("got %q, want %q", string(data), "persistent")
	}
}

func TestPath(t *testing.T) {
	s, _ := New(t.TempDir())
	id, _ := s.Save([]byte("data"), TypeImage, "image/png", "test.png")

	path := s.Path(id)
	if path == "" {
		t.Error("expected non-empty path")
	}
}
