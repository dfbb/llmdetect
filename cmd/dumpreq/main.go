package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ironarmor/llmdetect/internal/provider"
)

func main() {
	provider.OverrideCLIVersion("2.1.112")
	a := &provider.ClaudeCodeAdapter{}
	const token = "cr_f3cfdffec5bb90df1d524df8dad9cc436aabb779e035b388dc2e70b7e3c32e7f"
	const model = "claude-sonnet-4-6"
	const prompt = "Perform a web search for the query: software development developer tools news April 20 2026"

	body, err := a.BuildRequest(model, prompt, 32000)
	if err != nil {
		fmt.Fprintln(os.Stderr, err); os.Exit(1)
	}

	// Print the user_id area
	idx := bytes.Index(body, []byte("user_id"))
	if idx >= 0 {
		fmt.Printf("user_id area bytes: %q\n", body[idx:idx+120])
	}

	hdrs := a.HeadersForModel(token, model)
	req, _ := http.NewRequest("POST", "https://claude.kg83.org/api/v1/messages", bytes.NewReader(body))
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err); os.Exit(1)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	fmt.Printf("HTTP %d: %s\n", resp.StatusCode, raw[:min(len(raw), 300)])
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
