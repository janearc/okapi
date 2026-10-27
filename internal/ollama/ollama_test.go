package ollama_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/ollama"
	"github.com/janearc/okapi/internal/picked"
	"go.uber.org/mock/gomock"
)

// chatModel is the model every test here asks for.
const chatModel = "m"

// sentRequest is a gomock matcher that accepts any request and keeps
// it, so a test can read what was sent after the call.
type sentRequest struct {
	request *http.Request
	body    map[string]any
}

// Matches keeps the request and reads its body as json.
func (sent *sentRequest) Matches(value any) bool {
	request, ok := value.(*http.Request)
	if !ok {
		return false
	}
	sent.request = request
	if request.Body == nil {
		return true
	}
	return json.NewDecoder(request.Body).Decode(&sent.body) == nil
}

// String says what the matcher accepts.
func (sent *sentRequest) String() string { return "any request" }

// answering is a response with the status and body.
func answering(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// clientFor is a client for the endpoint at url that sends through doer.
func clientFor(t *testing.T, url string, with ollama.Doer) *ollama.Client {
	t.Helper()
	t.Setenv("OKAPI_OLLAMA", url)
	t.Setenv("OKAPI_MODEL", chatModel)
	return ollama.FromEnv(with)
}

// chatting is a client whose one request is answered with the status
// and body, and the request it will have sent.
func chatting(t *testing.T, status int, body string) (
	*ollama.Client, *sentRequest,
) {
	t.Helper()
	doer := NewMockDoer(gomock.NewController(t))
	sent := &sentRequest{}
	doer.EXPECT().Do(sent).Return(answering(status, body), nil)
	return clientFor(t, "http://test/api/chat", doer), sent
}

// the request names the model and asks for one answer. It asks for no
// thinking aloud at a low temperature. It carries the turns in order.
func TestChatSends(t *testing.T) {
	client, sent := chatting(t, http.StatusOK,
		`{"message":{"content":"<think>hm</think>\n// yes \n"}}`)
	text, err := client.Chat([]ollama.Message{
		ollama.System("rules"), ollama.User("q"), ollama.Assistant("a"),
	})
	if err != nil || text != "// yes" {
		t.Fatalf("got %q, %v", text, err)
	}
	if sent.request.Method != http.MethodPost ||
		sent.request.URL.String() != "http://test/api/chat" {
		t.Errorf("sent %s %s", sent.request.Method, sent.request.URL)
	}
	body := sent.body
	if body["model"] != chatModel || body["stream"] != false ||
		body["think"] != false {
		t.Errorf("sent %v", body)
	}
	options, _ := body["options"].(map[string]any)
	if options["temperature"] != picked.Value("temperature") {
		t.Errorf("options %v", options)
	}
	turns, _ := body["messages"].([]any)
	want := []string{"system rules", "user q", "assistant a"}
	var said []string
	for _, turn := range turns {
		fields, _ := turn.(map[string]any)
		role, _ := fields["role"].(string)
		content, _ := fields["content"].(string)
		said = append(said, role+" "+content)
	}
	if strings.Join(said, "|") != strings.Join(want, "|") {
		t.Errorf("turns %q, want %q", said, want)
	}
}

// statuses other than 200 are errors naming the status and the body.
func TestChatStatus(t *testing.T) {
	client, _ := chatting(t, http.StatusNotFound,
		`{"error":"no such model"}`+"\n")
	_, err := client.Chat(nil)
	if err == nil || !strings.Contains(err.Error(), "Not Found") ||
		!strings.HasSuffix(err.Error(), `{"error":"no such model"}`) {
		t.Errorf("got %v", err)
	}
}

// replies that are not json are said so, and replies that carry an
// error give that error.
func TestChatBadReplies(t *testing.T) {
	client, _ := chatting(t, http.StatusOK, "not json")
	if _, err := client.Chat(nil); err == nil ||
		!strings.Contains(err.Error(), "bad reply") {
		t.Errorf("not json: %v", err)
	}
	client, _ = chatting(t, http.StatusOK, `{"error":"out of memory"}`)
	if _, err := client.Chat(nil); err == nil ||
		err.Error() != "ollama: out of memory" {
		t.Errorf("error field: %v", err)
	}
}

// replies cut short are errors, even when what arrived reads as a
// reply.
func TestChatTruncated(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	cut := answering(http.StatusOK, "")
	head := strings.NewReader(`{"message":{"content":"hi"}}`)
	broken := iotest.ErrReader(io.ErrUnexpectedEOF)
	body := io.MultiReader(head, broken)
	cut.Body = io.NopCloser(body)
	doer.EXPECT().Do(gomock.Any()).Return(cut, nil)
	text, err := clientFor(t, "http://test/api/chat", doer).Chat(nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) || text != "" {
		t.Errorf("got %q, %v", text, err)
	}
}

// endpoints that cannot be reached say to check ollama is running.
func TestChatUnreachable(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	doer.EXPECT().Do(gomock.Any()).Return(nil, errors.New("refused"))
	client := clientFor(t, "http://test/api/chat", doer)
	_, err := client.Chat(nil)
	if err == nil || !strings.Contains(err.Error(), "is ollama running") ||
		!strings.Contains(err.Error(), "refused") {
		t.Errorf("unreachable: %v", err)
	}
}

// urls that cannot be requests do not reach the network, and do not
// say to check ollama.
func TestChatBadURLSendsNothing(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	client := clientFor(t, "http://a\x7fb/", doer)
	_, err := client.Chat(nil)
	if err == nil || strings.Contains(err.Error(), "is ollama running") {
		t.Errorf("bad url: %v", err)
	}
}

// up asks the tags endpoint and is true only for a 200.
func TestUp(t *testing.T) {
	statuses := map[int]bool{
		http.StatusOK:                  true,
		http.StatusNoContent:           false,
		http.StatusInternalServerError: false,
	}
	for status, want := range statuses {
		doer := NewMockDoer(gomock.NewController(t))
		sent := &sentRequest{}
		doer.EXPECT().Do(sent).Return(answering(status, ""), nil)
		client := clientFor(t, "http://test/api/chat", doer)
		if got := client.Up(); got != want {
			t.Errorf("status %d: up %v, want %v", status, got, want)
		}
		if sent.request.Method != http.MethodGet ||
			sent.request.URL.Path != "/api/tags" {
			t.Errorf("asked %s %s", sent.request.Method, sent.request.URL)
		}
	}
}

// up gives up on an endpoint that never answers, after two seconds:
// the request it sends carries that deadline.
func TestUpGivesUp(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	sent := &sentRequest{}
	doer.EXPECT().Do(sent).Return(nil, errors.New("deadline"))
	client := clientFor(t, "http://test/api/chat", doer)
	if client.Up() {
		t.Error("an endpoint that fails is not up")
	}
	deadline, has := sent.request.Context().Deadline()
	left := time.Until(deadline)
	allowed := picked.Span("up-wait", time.Second)
	if !has || left <= count.Nothing || left > allowed {
		t.Errorf("deadline %v in %v, want within %v", has, left, allowed)
	}
}

// urls that cannot be requests are not up, and send nothing.
func TestUpBadURLSendsNothing(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	if clientFor(t, "http://a\x7fb/", doer).Up() {
		t.Error("a url that is no url is not up")
	}
}

// the environment names the endpoint and the model; blank keeps the
// defaults; WithModel changes only the model.
func TestFromEnv(t *testing.T) {
	t.Setenv("OKAPI_OLLAMA", "")
	t.Setenv("OKAPI_MODEL", "")
	doer := NewMockDoer(gomock.NewController(t))
	client := ollama.FromEnv(doer)
	if client.URL() != "http://localhost:11434/api/chat" ||
		client.Model() != "qwen3-14b-12k" {
		t.Errorf("defaults: %s %s", client.URL(), client.Model())
	}
	client = clientFor(t, "http://127.0.0.1:1/x", doer)
	if client.URL() != "http://127.0.0.1:1/x" || client.Model() != chatModel {
		t.Errorf("from env: %s %s", client.URL(), client.Model())
	}
	other := client.WithModel("big")
	if other.URL() != client.URL() || other.Model() != "big" ||
		client.Model() != chatModel {
		t.Errorf("with model: %s %s", other.URL(), other.Model())
	}
}

// WithModel keeps the doer, so the model changes and the wire does not.
func TestWithModelKeepsTheDoer(t *testing.T) {
	doer := NewMockDoer(gomock.NewController(t))
	sent := &sentRequest{}
	doer.EXPECT().Do(sent).Return(answering(http.StatusOK,
		`{"message":{"content":"ok"}}`), nil)
	client := clientFor(t, "http://test/api/chat", doer)
	text, err := client.WithModel("big").Chat(nil)
	if err != nil || text != "ok" || sent.body["model"] != "big" {
		t.Errorf("got %q, %v, %v", text, err, sent.body)
	}
}

// the network doer gives up on an answer after the picked time.
func TestHTTPGivesUpAfterThePickedTime(t *testing.T) {
	network, ok := ollama.HTTP().(*http.Client)
	want := picked.Span("answer-wait", time.Minute)
	if !ok || network.Timeout != want {
		t.Errorf("the network doer is %v, want a client that waits %v",
			ollama.HTTP(), want)
	}
}
