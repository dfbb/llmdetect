// internal/provider/adapter_test.go
package provider_test

import (
	"testing"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func TestAdapterFromType_OpenAI(t *testing.T) {
	a, err := provider.AdapterFromType(provider.ProviderOpenAI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Type() != provider.ProviderOpenAI {
		t.Errorf("got %v, want %v", a.Type(), provider.ProviderOpenAI)
	}
}

func TestAdapterFromType_Anthropic(t *testing.T) {
	a, err := provider.AdapterFromType(provider.ProviderAnthropic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Type() != provider.ProviderAnthropic {
		t.Errorf("got %v, want %v", a.Type(), provider.ProviderAnthropic)
	}
}

func TestAdapterFromType_Unknown(t *testing.T) {
	_, err := provider.AdapterFromType("gemini")
	if err == nil {
		t.Fatal("expected error for unknown provider type")
	}
}
