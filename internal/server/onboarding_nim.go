package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultNIMBaseURL = "https://integrate.api.nvidia.com/v1"
	defaultNIMModel   = "meta/llama-3.1-8b-instruct"
	nimResponseLimit  = 1 << 20
)

const onboardingSystemPrompt = `你是“今天吃什么”的首次使用访谈助手。用简短自然的中文追问 Eater 喜欢的具体 Dish，并在信息足够时整理偏好。
只输出一个 JSON 对象，不要 Markdown 或额外文字：
{"reply":"给 Eater 的自然回复","complete":false,"preferences":[{"dish_name":"具体菜名","weight":1}]}
complete 仅在 Eater 已明确给出至少一道具体菜名且偏好足够建立初始 Candidate pool 时为 true。
weight 必须在 1 到 5 之间，越喜欢越高。不要创造 Dish ID；服务端只会接受 Catalog 中名称完全匹配的 Dish。`

type NIMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

type onboardingNIM interface {
	Respond(context.Context, []onboardingMessage) (nimInterviewResult, error)
}

type unavailableNIM struct{}

func (unavailableNIM) Respond(
	context.Context,
	[]onboardingMessage,
) (nimInterviewResult, error) {
	return nimInterviewResult{}, errors.New("NVIDIA NIM is not configured")
}

type httpNIM struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

type nimInterviewResult struct {
	Reply       string          `json:"reply"`
	Complete    bool            `json:"complete"`
	Preferences []nimPreference `json:"preferences"`
}

type nimPreference struct {
	DishName string  `json:"dish_name"`
	Weight   float64 `json:"weight"`
}

func newNIMAdapter(config *NIMConfig) (onboardingNIM, error) {
	if config == nil || config.APIKey == "" {
		return unavailableNIM{}, nil
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultNIMBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("NIM BaseURL must be an absolute HTTPS URL")
	}
	hostIP := net.ParseIP(parsed.Hostname())
	isLoopback := parsed.Hostname() == "localhost" || (hostIP != nil && hostIP.IsLoopback())
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopback) {
		return nil, errors.New("NIM BaseURL must use HTTPS outside loopback")
	}
	model := config.Model
	if model == "" {
		model = defaultNIMModel
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &httpNIM{
		apiKey:   config.APIKey,
		endpoint: strings.TrimRight(baseURL, "/") + "/chat/completions",
		model:    model,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (n *httpNIM) Respond(
	context context.Context,
	messages []onboardingMessage,
) (nimInterviewResult, error) {
	providerMessages := make([]onboardingMessage, 0, len(messages)+1)
	providerMessages = append(providerMessages, onboardingMessage{
		Role:    "system",
		Content: onboardingSystemPrompt,
	})
	providerMessages = append(providerMessages, messages...)
	body, err := json.Marshal(struct {
		Model       string              `json:"model"`
		Messages    []onboardingMessage `json:"messages"`
		Temperature float64             `json:"temperature"`
		MaxTokens   int                 `json:"max_tokens"`
		Stream      bool                `json:"stream"`
	}{
		Model:       n.model,
		Messages:    providerMessages,
		Temperature: 0.2,
		MaxTokens:   800,
		Stream:      false,
	})
	if err != nil {
		return nimInterviewResult{}, fmt.Errorf("encode NIM request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		context,
		http.MethodPost,
		n.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nimInterviewResult{}, fmt.Errorf("create NIM request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+n.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := n.client.Do(request)
	if err != nil {
		return nimInterviewResult{}, fmt.Errorf("call NIM: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, nimResponseLimit+1))
	if err != nil {
		return nimInterviewResult{}, fmt.Errorf("read NIM response: %w", err)
	}
	if len(responseBody) > nimResponseLimit {
		return nimInterviewResult{}, errors.New("NIM response is too large")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nimInterviewResult{}, fmt.Errorf("NIM returned HTTP %d", response.StatusCode)
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return nimInterviewResult{}, errors.New("NIM returned invalid completion JSON")
	}
	if len(completion.Choices) == 0 {
		return nimInterviewResult{}, errors.New("NIM returned no completion choice")
	}

	var result nimInterviewResult
	if err := json.Unmarshal(
		[]byte(strings.TrimSpace(completion.Choices[0].Message.Content)),
		&result,
	); err != nil {
		return nimInterviewResult{}, errors.New("NIM returned invalid interview JSON")
	}
	result.Reply = strings.TrimSpace(result.Reply)
	if result.Reply == "" || utf8.RuneCountInString(result.Reply) > 2_000 {
		return nimInterviewResult{}, errors.New("NIM returned an invalid interview reply")
	}
	if len(result.Preferences) > 20 {
		return nimInterviewResult{}, errors.New("NIM returned too many preferences")
	}
	for _, preference := range result.Preferences {
		if preference.DishName != strings.TrimSpace(preference.DishName) ||
			preference.DishName == "" ||
			utf8.RuneCountInString(preference.DishName) > 100 {
			return nimInterviewResult{}, errors.New("NIM returned an invalid Dish name")
		}
	}
	return result, nil
}
