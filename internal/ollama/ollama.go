// Package ollama talks to a model that ollama serves on this machine,
// through its chat endpoint.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/picked"
)

// defaultModel is the model drafts are asked of.
const defaultModel = "qwen3-14b-12k"

// defaultURL is ollama's chat endpoint on this machine.
const defaultURL = "http://localhost:11434/api/chat"

// answerWait is how long one answer may take.
func answerWait() time.Duration {
	return picked.Span("answer-wait", time.Minute)
}

// upWait is how long Up waits for the endpoint to answer.
func upWait() time.Duration {
	return picked.Span("up-wait", time.Second)
}

// Doer sends one request and returns the response. Each client takes
// the one it is given, so a test hands it a mock.
type Doer interface {
	// Do sends the request.
	Do(request *http.Request) (*http.Response, error)
}

// Client talks to one endpoint about one model.
type Client struct {
	url   string
	model string
	doer  Doer
}

// HTTP is a Doer that sends over the network, and gives up on an answer
// that takes too long.
func HTTP() Doer {
	return &http.Client{Timeout: answerWait()}
}

// FromEnv is the client the environment names, sending through doer.
// OKAPI_OLLAMA names the endpoint and OKAPI_MODEL names the model.
// Blank keeps the default of either.
func FromEnv(doer Doer) *Client {
	client := &Client{
		url: defaultURL, model: defaultModel,
		doer: doer,
	}
	if url := os.Getenv("OKAPI_OLLAMA"); url != "" {
		client.url = url
	}
	if model := os.Getenv("OKAPI_MODEL"); model != "" {
		client.model = model
	}
	return client
}

// WithModel is the same client asking a different model.
func (client *Client) WithModel(model string) *Client {
	return &Client{url: client.url, model: model, doer: client.doer}
}

// URL is the endpoint the client talks to.
func (client *Client) URL() string { return client.url }

// Model is the model the client asks.
func (client *Client) Model() string { return client.model }

// Message is one turn of a chat: who said it, and what.
type Message struct{ role, content string }

// System is a message that sets the rules.
func System(text string) Message { return Message{"system", text} }

// User is a message from the person asking.
func User(text string) Message { return Message{"user", text} }

// Assistant is a message in the model's own turn, given as an example.
func Assistant(text string) Message { return Message{"assistant", text} }

// thinking matches the reasoning a model may show before its answer.
var thinking = regexp.MustCompile(`(?s)<think>.*?</think>`)

// Chat sends the messages and returns the model's answer. The answer
// is trimmed and any reasoning is removed. The model is asked not to
// think aloud and to keep a low temperature.
func (client *Client) Chat(messages []Message) (string, error) {
	turns := make([]map[string]string, count.Nothing, len(messages))
	for _, message := range messages {
		turns = append(turns, map[string]string{
			"role": message.role, "content": message.content,
		})
	}
	warmth := picked.Value("temperature")
	// Strings, booleans and a number always marshal.
	body, _ := json.Marshal(map[string]any{
		"model":    client.model,
		"messages": turns,
		"stream":   false,
		"think":    false,
		"options":  map[string]any{"temperature": warmth},
	})
	req, err := http.NewRequest(http.MethodPost, client.url,
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.doer.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: %w (is ollama running?)", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		said := strings.TrimSpace(string(raw))
		return "", fmt.Errorf("ollama: %s: %s", resp.Status, said)
	}
	return answer(raw)
}

// answer is the text of a chat reply, or the error the reply carries.
func answer(raw []byte) (string, error) {
	var reply struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return "", fmt.Errorf("ollama: bad reply: %w", err)
	}
	if reply.Error != "" {
		return "", fmt.Errorf("ollama: %s", reply.Error)
	}
	text := thinking.ReplaceAllString(reply.Message.Content, "")
	return strings.TrimSpace(text), nil
}

// Up says whether the endpoint answers at all, within two seconds.
func (client *Client) Up() bool {
	tags := strings.TrimSuffix(client.url, "/api/chat") + "/api/tags"
	ctx, cancel := context.WithTimeout(context.Background(), upWait())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tags, nil)
	if err != nil {
		return false
	}
	resp, err := client.doer.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
