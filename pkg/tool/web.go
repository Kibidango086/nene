package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type WebSearchTool struct {
	parameters json.RawMessage
}

func NewWebSearchTool() *WebSearchTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (default: 5)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
		},
		"required": []string{"query"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WebSearchTool{parameters: paramsJSON}
}

func (t *WebSearchTool) Name() string { return "websearch" }
func (t *WebSearchTool) Description() string {
	return "Search the web using DuckDuckGo. Returns search results with titles, URLs, and snippets. Use this to find current information, news, or any content beyond your knowledge cutoff."
}
func (t *WebSearchTool) Parameters() json.RawMessage { return t.parameters }

type webSearchArgs struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
}

func (t *WebSearchTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a webSearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return NewApproval("Agent wants to search the web", "Search: "+a.Query), nil
}

func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a webSearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.Query == "" {
		return ErrorResult("query is required"), nil
	}

	if a.NumResults <= 0 {
		a.NumResults = 5
	}

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(a.Query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ErrorResult("failed to create request: " + err.Error()), nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult("request failed: " + err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorResult("failed to read response: " + err.Error()), nil
	}

	return OkResult(t.extractResults(string(body), a.NumResults, a.Query)), nil
}

func (t *WebSearchTool) extractResults(html string, count int, query string) string {
	reLink := regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	matches := reLink.FindAllStringSubmatch(html, count+5)

	if len(matches) == 0 {
		return fmt.Sprintf("No results found for: %s", query)
	}

	reSnippet := regexp.MustCompile(`<a class="result__snippet[^"]*".*?>([\s\S]*?)</a>`)
	snippetMatches := reSnippet.FindAllStringSubmatch(html, count+5)

	var lines []string
	lines = append(lines, fmt.Sprintf("Search results for: %s", query))

	maxItems := min(len(matches), count)

	for i := 0; i < maxItems; i++ {
		urlStr := matches[i][1]
		title := stripTags(matches[i][2])
		title = strings.TrimSpace(title)

		if strings.Contains(urlStr, "uddg=") {
			if u, err := url.QueryUnescape(urlStr); err == nil {
				idx := strings.Index(u, "uddg=")
				if idx != -1 {
					urlStr = u[idx+5:]
				}
			}
		}

		lines = append(lines, fmt.Sprintf("\n%d. %s", i+1, title))
		lines = append(lines, fmt.Sprintf("   URL: %s", urlStr))

		if i < len(snippetMatches) {
			snippet := stripTags(snippetMatches[i][1])
			snippet = strings.TrimSpace(snippet)
			if snippet != "" {
				lines = append(lines, fmt.Sprintf("   %s", snippet))
			}
		}
	}

	return strings.Join(lines, "\n")
}

type WebFetchTool struct {
	parameters json.RawMessage
}

func NewWebFetchTool() *WebFetchTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch content from",
			},
			"max_chars": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum characters to return (default: 10000)",
				"minimum":     1000.0,
				"maximum":     50000.0,
			},
		},
		"required": []string{"url"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WebFetchTool{parameters: paramsJSON}
}

func (t *WebFetchTool) Name() string { return "webfetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch content from a URL. Extracts readable text from web pages. Use this to get detailed content from a specific URL found via web search."
}
func (t *WebFetchTool) Parameters() json.RawMessage { return t.parameters }

type webFetchArgs struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars"`
}

func (t *WebFetchTool) MakeApproval(args json.RawMessage) (*Approval, error) {
	var a webFetchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return NewApproval("Agent wants to fetch web content", "Fetch: "+a.URL), nil
}

func (t *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var a webFetchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if a.URL == "" {
		return ErrorResult("url is required"), nil
	}

	if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		return ErrorResult("URL must start with http:// or https://"), nil
	}

	if a.MaxChars <= 0 {
		a.MaxChars = 10000
	}

	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return ErrorResult("failed to create request: " + err.Error()), nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.1")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult("request failed: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("request failed with status: %d", resp.StatusCode)), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return ErrorResult("failed to read response: " + err.Error()), nil
	}

	content := string(body)
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") || looksLikeHTML(content) {
		content = extractTextFromHTML(content)
	}

	if len(content) > a.MaxChars {
		content = content[:a.MaxChars] + "\n... (truncated)"
	}

	return OkResult(fmt.Sprintf("Content from %s:\n\n%s", a.URL, content)), nil
}

func stripTags(content string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(content, "")
}

func looksLikeHTML(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "<!DOCTYPE") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<html") ||
		(strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">"))
}

func extractTextFromHTML(html string) string {
	result := html

	for {
		start := strings.Index(strings.ToLower(result), "<script")
		if start == -1 {
			break
		}
		end := strings.Index(strings.ToLower(result[start:]), "</script>")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+9:]
	}

	for {
		start := strings.Index(strings.ToLower(result), "<style")
		if start == -1 {
			break
		}
		end := strings.Index(strings.ToLower(result[start:]), "</style>")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+8:]
	}

	var output strings.Builder
	inTag := false
	for _, r := range result {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			output.WriteRune(r)
		}
	}

	text := output.String()

	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}
