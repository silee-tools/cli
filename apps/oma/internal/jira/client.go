package jira

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

	netrc "github.com/jdx/go-netrc"
)

const maxResponseBytes = 8 << 20

func CredentialsFromNetrc(path, host string) (credentials Credentials, resultErr error) {
	defer func() {
		if recover() != nil {
			credentials = Credentials{}
			resultErr = errors.New("read netrc: invalid format")
		}
	}()
	if strings.TrimSpace(path) == "" || strings.TrimSpace(host) == "" {
		return Credentials{}, errors.New("netrc path and host are required")
	}
	file, err := netrc.Parse(path)
	if err != nil {
		return Credentials{}, boundedError("read netrc", err)
	}
	machine := file.Machine(host)
	if machine == nil {
		return Credentials{}, fmt.Errorf("netrc has no machine for %q", host)
	}
	credentials = Credentials{
		Username: machine.Get("login"),
		Password: machine.Get("password"),
	}
	if credentials.Username == "" || credentials.Password == "" {
		return Credentials{}, fmt.Errorf("netrc machine %q is missing login or password", host)
	}
	return credentials, nil
}

func NewClient(baseURL string, httpClient *http.Client, credentials Credentials) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("jira base URL must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("jira base URL must use HTTP or HTTPS")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return nil, errors.New("jira base URL must use HTTPS unless the host is loopback")
	}
	if parsed.User != nil {
		return nil, errors.New("jira base URL must not contain credentials")
	}
	if credentials.Username == "" || credentials.Password == "" {
		return nil, errors.New("jira credentials are incomplete")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	clientCopy := *httpClient
	clientCopy.Jar = nil
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return &Client{
		baseURL:     strings.TrimRight(parsed.String(), "/"),
		httpClient:  &clientCopy,
		credentials: credentials,
	}, nil
}

func (c *Client) FetchIssue(ctx context.Context, key string) (Issue, []byte, error) {
	raw, err := c.request(ctx, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key), url.Values{"fields": {"*all"}}, nil, http.StatusOK)
	if err != nil {
		return Issue{}, nil, err
	}
	var response struct {
		Key    string                     `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return Issue{}, nil, boundedError("decode Jira issue", err)
	}
	if response.Key == "" || response.Fields == nil {
		return Issue{}, nil, errors.New("decode Jira issue: required key or fields missing")
	}
	issue := Issue{Key: response.Key, CustomFields: make(map[string]json.RawMessage)}
	if err := decodeField(response.Fields, "summary", &issue.Summary); err != nil {
		return Issue{}, nil, err
	}
	if err := decodeField(response.Fields, "status", &issue.Status); err != nil {
		return Issue{}, nil, err
	}
	if err := validateStatus(issue.Status, "issue status"); err != nil {
		return Issue{}, nil, err
	}
	if assignee, ok := response.Fields["assignee"]; ok && string(assignee) != "null" {
		if err := json.Unmarshal(assignee, &issue.Assignee); err != nil {
			return Issue{}, nil, boundedError("decode Jira field assignee", err)
		}
	}
	if description, ok := response.Fields["description"]; ok && string(description) != "null" {
		var document adfNode
		if err := json.Unmarshal(description, &document); err != nil {
			return Issue{}, nil, boundedError("decode Jira field description", err)
		}
		issue.DescriptionText = strings.TrimRight(renderADF(document), "\n")
	}
	for name, value := range response.Fields {
		switch name {
		case "summary", "description", "status", "assignee":
			continue
		default:
			issue.CustomFields[name] = append(json.RawMessage(nil), value...)
		}
	}
	return issue, append([]byte(nil), raw...), nil
}

func (c *Client) Myself(ctx context.Context) (User, error) {
	raw, err := c.request(ctx, http.MethodGet, "/rest/api/3/myself", nil, nil, http.StatusOK)
	if err != nil {
		return User{}, err
	}
	var user User
	if err := json.Unmarshal(raw, &user); err != nil {
		return User{}, boundedError("decode current Jira user", err)
	}
	if user.AccountID == "" || user.DisplayName == "" {
		return User{}, errors.New("decode current Jira user: accountId or displayName missing")
	}
	return user, nil
}

func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	raw, err := c.request(ctx, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key)+"/transitions", url.Values{"expand": {"transitions.fields"}}, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response struct {
		Transitions []Transition `json:"transitions"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, boundedError("decode Jira transitions", err)
	}
	if response.Transitions == nil {
		return nil, errors.New("decode Jira transitions: transitions missing")
	}
	for index, transition := range response.Transitions {
		if transition.ID == "" {
			return nil, fmt.Errorf("decode Jira transition %d: id missing", index)
		}
		if err := validateStatus(transition.To, fmt.Sprintf("transition %d destination", index)); err != nil {
			return nil, err
		}
	}
	return response.Transitions, nil
}

func (c *Client) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	if len(fields) == 0 {
		return errors.New("jira field update must contain at least one changed field")
	}
	body, err := json.Marshal(struct {
		Fields map[string]any `json:"fields"`
	}{Fields: fields})
	if err != nil {
		return boundedError("encode Jira field update", err)
	}
	_, err = c.request(ctx, http.MethodPut, "/rest/api/3/issue/"+url.PathEscape(key), nil, body, http.StatusNoContent)
	return err
}

func (c *Client) ApplyTransition(ctx context.Context, key, transitionID string) error {
	if transitionID == "" {
		return errors.New("jira transition ID is required")
	}
	body, err := json.Marshal(struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}{Transition: struct {
		ID string `json:"id"`
	}{ID: transitionID}})
	if err != nil {
		return boundedError("encode Jira transition", err)
	}
	_, err = c.request(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/transitions", nil, body, http.StatusNoContent)
	return err
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body []byte, expectedStatus int) ([]byte, error) {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, boundedError("create Jira request", err)
	}
	request.SetBasicAuth(c.credentials.Username, c.credentials.Password)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, boundedError("perform Jira request", err)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	closeErr := response.Body.Close()
	if err != nil {
		return nil, boundedError("read Jira response", err)
	}
	if closeErr != nil {
		return nil, boundedError("close Jira response", closeErr)
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("jira response exceeded the size limit")
	}
	if response.StatusCode != expectedStatus {
		return nil, fmt.Errorf("jira request returned HTTP %d, expected HTTP %d", response.StatusCode, expectedStatus)
	}
	return raw, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func decodeField(fields map[string]json.RawMessage, name string, destination any) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("decode Jira issue: required field %s missing", name)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return boundedError("decode Jira field "+name, err)
	}
	return nil
}

func validateStatus(status Status, context string) error {
	if status.ID == "" || status.Name == "" || status.CategoryKey == "" {
		return fmt.Errorf("decode Jira %s: id, name, or status category missing", context)
	}
	return nil
}

func (status *Status) UnmarshalJSON(raw []byte) error {
	var value struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	status.ID = value.ID
	status.Name = value.Name
	status.CategoryKey = value.StatusCategory.Key
	return nil
}

type adfNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text"`
	Content []adfNode `json:"content"`
}

func renderADF(node adfNode) string {
	if node.Type == "text" {
		return node.Text
	}
	if node.Type == "hardBreak" {
		return "\n"
	}
	var builder strings.Builder
	for _, child := range node.Content {
		builder.WriteString(renderADF(child))
	}
	if node.Type == "paragraph" || node.Type == "heading" {
		builder.WriteByte('\n')
	}
	return builder.String()
}

func boundedError(operation string, err error) error {
	message := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < ' ' {
			return -1
		}
		return r
	}, err.Error())
	const maxDetail = 320
	if len(message) > maxDetail {
		message = message[:maxDetail] + "..."
	}
	return fmt.Errorf("%s: %s", operation, message)
}
