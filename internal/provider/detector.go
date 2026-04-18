package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrProviderUndetectable is returned when neither OpenAI nor Anthropic format
// returns HTTP 200 from the target endpoint.
var ErrProviderUndetectable = errors.New("provider undetectable: neither openai nor anthropic format responded with 200")

var yamlMu sync.Mutex

// Detect probes baseURL to determine its API format, writes the result to yamlPath
// (matched by endpointURL), and returns the matching Adapter.
// If baseURL contains "openrouter.ai", OpenAIAdapter is returned without probing.
// Set yamlPath="" to skip writeback.
func Detect(ctx context.Context, baseURL, apiKey, model, yamlPath, endpointURL string, timeout time.Duration) (Adapter, error) {
	if strings.Contains(baseURL, "openrouter.ai") {
		return &OpenAIAdapter{}, nil
	}

	client := &http.Client{Timeout: timeout}

	for _, a := range []Adapter{&OpenAIAdapter{}, &AnthropicAdapter{}} {
		if probeAdapter(ctx, client, baseURL, apiKey, model, a) {
			if yamlPath != "" {
				yamlMu.Lock()
				if err := writeProviderToYAML(yamlPath, endpointURL, a.Type()); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not write provider to %s: %v\n", yamlPath, err)
				}
				yamlMu.Unlock()
			}
			return a, nil
		}
	}
	return nil, ErrProviderUndetectable
}

func probeAdapter(ctx context.Context, client *http.Client, baseURL, apiKey, model string, a Adapter) bool {
	body, err := a.BuildRequest(model, "hi", 1)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+a.RequestPath(), bytes.NewReader(body))
	if err != nil {
		return false
	}
	for k, v := range a.Headers(apiKey) {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// writeProviderToYAML updates the provider field of the endpoint with matching URL in yamlPath.
// Preserves all other fields, ordering, and comments using yaml.Node.
func writeProviderToYAML(yamlPath, endpointURL string, p ProviderType) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("empty YAML document in %s", yamlPath)
	}
	if !setProviderInNode(root.Content[0], endpointURL, string(p)) {
		return fmt.Errorf("endpoint URL %s not found in %s", endpointURL, yamlPath)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(yamlPath, out, 0644)
}

// setProviderInNode recursively walks node looking for a mapping with url==targetURL,
// then adds or updates its provider field. Returns true if found.
func setProviderInNode(node *yaml.Node, targetURL, providerVal string) bool {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "url" && node.Content[i+1].Value == targetURL {
				setMappingKey(node, "provider", providerVal)
				return true
			}
		}
		for i := 1; i < len(node.Content); i += 2 {
			if setProviderInNode(node.Content[i], targetURL, providerVal) {
				return true
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if setProviderInNode(child, targetURL, providerVal) {
				return true
			}
		}
	}
	return false
}

// setMappingKey updates an existing key's value in a mapping node,
// or appends a new key-value pair if the key does not exist.
func setMappingKey(node *yaml.Node, key, value string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}
