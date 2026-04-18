package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

var ErrCorrupted = errors.New("cache file is corrupted")
var ErrNotFound = errors.New("cache file not found")

type BorderInput struct {
	Prompt               string         `json:"prompt"`
	OfficialDistribution map[string]int `json:"official_distribution"`
}

type CacheFile struct {
	Model        string        `json:"model"`
	OfficialURL  string        `json:"official_url"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	BorderInputs []BorderInput `json:"border_inputs"`
}

type Cache struct {
	path string
}

func New(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) Save(cf *CacheFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0644); err != nil {
		return fmt.Errorf("write cache %s: %w", c.path, err)
	}
	return nil
}

func (c *Cache) Load() (*CacheFile, error) {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		os.Remove(c.path)
		return nil, ErrCorrupted
	}
	return &cf, nil
}

// IsExpired returns true if the cache file does not exist or its ExpiresAt has passed.
func (c *Cache) IsExpired() bool {
	cf, err := c.Load()
	if err != nil {
		return true
	}
	return time.Now().After(cf.ExpiresAt)
}

// TimeNow returns the current UTC time. Provided for use in test helpers.
func TimeNow() time.Time { return time.Now().UTC() }
