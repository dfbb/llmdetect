package cache_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ironarmor/llmdetect/internal/cache"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")

	c := cache.New(path)
	original := &cache.CacheFile{
		Model:       "gpt-4o",
		OfficialURL: "https://api.openai.com/v1",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		ExpiresAt:   time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		BorderInputs: []cache.BorderInput{
			{Prompt: "hello", OfficialDistribution: map[string]int{"world": 5, "there": 3}},
		},
	}

	if err := c.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := c.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Model != original.Model {
		t.Errorf("Model: got %q want %q", loaded.Model, original.Model)
	}
	if len(loaded.BorderInputs) != 1 {
		t.Errorf("BorderInputs: got %d want 1", len(loaded.BorderInputs))
	}
}

func TestIsExpired_Fresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")
	c := cache.New(path)

	future := &cache.CacheFile{
		Model: "m", OfficialURL: "u",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if err := c.Save(future); err != nil {
		t.Fatal(err)
	}
	if c.IsExpired() {
		t.Error("fresh cache should not be expired")
	}
}

func TestIsExpired_Stale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")
	c := cache.New(path)

	past := &cache.CacheFile{
		Model: "m", OfficialURL: "u",
		CreatedAt: time.Now().Add(-2 * time.Hour).UTC(),
		ExpiresAt: time.Now().Add(-time.Hour).UTC(),
	}
	if err := c.Save(past); err != nil {
		t.Fatal(err)
	}
	if !c.IsExpired() {
		t.Error("stale cache should be expired")
	}
}

func TestCorruptedCacheDeletedAndReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cache")
	if err := os.WriteFile(path, []byte("{corrupted json"), 0644); err != nil {
		t.Fatal(err)
	}

	c := cache.New(path)
	_, err := c.Load()
	if !errors.Is(err, cache.ErrCorrupted) {
		t.Fatalf("expected ErrCorrupted, got: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("corrupted cache file should have been deleted")
	}
}
