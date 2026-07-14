package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	testUsername = "agent@example.com"
	testPassword = "reserved-test-token"
)

func TestClientFetchIssuePreservesRawAndParsesAllFields(t *testing.T) {
	response := []byte(`{"key":"OMA-42","fields":{"summary":"Prepare CLI","description":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"First line"}]},{"type":"paragraph","content":[{"type":"text","text":"Second"},{"type":"hardBreak"},{"type":"text","text":"line"}]}]},"status":{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}},"assignee":{"accountId":"acct-1","displayName":"Test User"},"customfield_10001":{"id":"product-a","value":"Product A"}}}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/OMA-42" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("fields"); got != "*all" {
			t.Errorf("fields = %q, want *all", got)
		}
		assertBasicAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, server.Client())
	issue, raw, err := client.FetchIssue(context.Background(), "OMA-42")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	if string(raw) != string(response) {
		t.Fatalf("raw response changed\ngot:  %s\nwant: %s", raw, response)
	}
	if issue.Key != "OMA-42" || issue.Summary != "Prepare CLI" {
		t.Fatalf("issue identity = %#v", issue)
	}
	if issue.DescriptionText != "First line\nSecond\nline" {
		t.Errorf("description = %q", issue.DescriptionText)
	}
	if issue.Status.CategoryKey != "indeterminate" || issue.Assignee == nil || issue.Assignee.AccountID != "acct-1" {
		t.Errorf("status/assignee = %#v / %#v", issue.Status, issue.Assignee)
	}
	if got := string(issue.CustomFields["customfield_10001"]); got != `{"id":"product-a","value":"Product A"}` {
		t.Errorf("custom field = %s", got)
	}
	if _, exists := issue.CustomFields["summary"]; exists {
		t.Error("fixed summary field leaked into CustomFields")
	}
}

func TestClientMyselfTransitionsAndWritesUseJiraContracts(t *testing.T) {
	type observedRequest struct {
		method string
		path   string
		query  url.Values
		body   []byte
	}
	requests := make(chan observedRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requests <- observedRequest{r.Method, r.URL.Path, r.URL.Query(), body}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_, _ = io.WriteString(w, `{"accountId":"acct-me","displayName":"Current User"}`)
		case "/rest/api/3/issue/OMA-42/transitions":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"transitions":[{"id":"21","name":"Start Progress","to":{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}}}]}`)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, server.Client())
	user, err := client.Myself(context.Background())
	if err != nil || user.AccountID != "acct-me" {
		t.Fatalf("Myself = %#v, %v", user, err)
	}
	transitions, err := client.Transitions(context.Background(), "OMA-42")
	if err != nil || len(transitions) != 1 || transitions[0].To.CategoryKey != "indeterminate" {
		t.Fatalf("Transitions = %#v, %v", transitions, err)
	}
	fields := map[string]any{"assignee": map[string]string{"accountId": "acct-me"}}
	if err := client.UpdateFields(context.Background(), "OMA-42", fields); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	if err := client.ApplyTransition(context.Background(), "OMA-42", "21"); err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}

	got := make([]observedRequest, 0, 4)
	for range 4 {
		got = append(got, <-requests)
	}
	if got[0].method != http.MethodGet || got[0].path != "/rest/api/3/myself" {
		t.Errorf("myself request = %#v", got[0])
	}
	if got[1].method != http.MethodGet || got[1].path != "/rest/api/3/issue/OMA-42/transitions" || got[1].query.Get("expand") != "transitions.fields" {
		t.Errorf("transitions request = %#v", got[1])
	}
	assertJSONEqual(t, got[2].body, []byte(`{"fields":{"assignee":{"accountId":"acct-me"}}}`))
	if got[2].method != http.MethodPut || got[2].path != "/rest/api/3/issue/OMA-42" {
		t.Errorf("update request = %#v", got[2])
	}
	assertJSONEqual(t, got[3].body, []byte(`{"transition":{"id":"21"}}`))
	if got[3].method != http.MethodPost || got[3].path != "/rest/api/3/issue/OMA-42/transitions" {
		t.Errorf("transition request = %#v", got[3])
	}
}

func TestClientErrorsAreBoundedAndDoNotLeakSecretsOrFullBodies(t *testing.T) {
	secretBody := strings.Repeat("sensitive-response-content-", 80)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, secretBody)
	}))
	client := newTestClient(t, server.URL, server.Client())
	_, _, err := client.FetchIssue(context.Background(), "OMA-42")
	assertSafeError(t, err, secretBody)
	server.Close()

	_, err = client.Transitions(context.Background(), "OMA-42")
	assertSafeError(t, err, secretBody)

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"key":`)
	}))
	defer malformed.Close()
	client = newTestClient(t, malformed.URL, malformed.Client())
	_, _, err = client.FetchIssue(context.Background(), "OMA-42")
	assertSafeError(t, err, `{"key":`)
}

func TestCredentialsFromNetrc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netrc")
	content := "machine jira.example.test login " + testUsername + " password " + testPassword + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials, err := CredentialsFromNetrc(path, "jira.example.test")
	if err != nil {
		t.Fatalf("CredentialsFromNetrc: %v", err)
	}
	if credentials.Username != testUsername || credentials.Password != testPassword {
		t.Fatal("credentials did not match the reserved test values")
	}
	_, err = CredentialsFromNetrc(path, "missing.example.test")
	if err == nil {
		t.Fatal("missing machine returned no error")
	}
}

func TestClientRejectsRedirectsWithoutMutatingCallerClient(t *testing.T) {
	var finalRequests atomic.Int32
	var sourceRequests atomic.Int32
	redirectStatuses := []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			finalRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"key":"OMA-42","fields":{}}`)
			return
		}
		index := int(sourceRequests.Add(1)) - 1
		status := http.StatusFound
		if index < len(redirectStatuses) {
			status = redirectStatuses[index]
		}
		http.Redirect(w, r, "/final", status)
	}))
	defer server.Close()

	callerClient := server.Client()
	client := newTestClient(t, server.URL, callerClient)
	for _, status := range redirectStatuses {
		_, _, err := client.FetchIssue(context.Background(), "OMA-42")
		assertSafeError(t, err, "/final")
		if !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
			t.Errorf("redirect error = %q, want HTTP %d", err, status)
		}
	}
	if got := finalRequests.Load(); got != 0 {
		t.Fatalf("Jira client followed redirect %d time(s)", got)
	}

	response, err := callerClient.Get(server.URL + "/source")
	if err != nil {
		t.Fatalf("caller client no longer follows redirects: %v", err)
	}
	_ = response.Body.Close()
	if got := finalRequests.Load(); got != 1 {
		t.Fatalf("caller client was mutated; final requests = %d", got)
	}
}

func TestClientDoesNotMutateCallerCookieJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "jira-session", Value: "server-value", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"key":"OMA-42","fields":{"summary":"Prepare CLI","status":{"id":"3","name":"In Progress","statusCategory":{"key":"indeterminate"}}}}`)
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	callerClient := server.Client()
	callerClient.Jar = jar
	client := newTestClient(t, server.URL, callerClient)
	if _, _, err := client.FetchIssue(context.Background(), "OMA-42"); err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if cookies := jar.Cookies(serverURL); len(cookies) != 0 {
		t.Fatalf("Jira client mutated caller cookie jar: %#v", cookies)
	}
}

func TestClientValidatesWriteSuccessStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		call   func(*Client) error
	}{
		{
			name:   "update fields",
			method: http.MethodPut,
			call: func(client *Client) error {
				return client.UpdateFields(context.Background(), "OMA-42", map[string]any{"summary": "changed"})
			},
		},
		{
			name:   "apply transition",
			method: http.MethodPost,
			call: func(client *Client) error {
				return client.ApplyTransition(context.Background(), "OMA-42", "21")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var finalRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != test.method {
					t.Errorf("method = %s, want %s", r.Method, test.method)
				}
				if r.URL.Path == "/final" {
					finalRequests.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.Redirect(w, r, "/final", http.StatusSeeOther)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, server.Client())
			err := test.call(client)
			assertSafeError(t, err, "/final")
			if got := finalRequests.Load(); got != 0 {
				t.Fatalf("write followed redirect %d time(s)", got)
			}

			htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, "<html>not Jira JSON</html>")
			}))
			defer htmlServer.Close()
			htmlClient := newTestClient(t, htmlServer.URL, htmlServer.Client())
			err = test.call(htmlClient)
			if err == nil {
				t.Fatal("HTTP 200 text/html was accepted as a Jira write success")
			}
		})
	}
}

func TestNewClientRejectsRemoteHTTPAndAllowsLoopback(t *testing.T) {
	for _, test := range []struct {
		baseURL string
		wantErr bool
	}{
		{baseURL: "https://jira.example.test"},
		{baseURL: "http://localhost:8080"},
		{baseURL: "http://127.0.0.1:8080"},
		{baseURL: "http://[::1]:8080"},
		{baseURL: "http://jira.example.test", wantErr: true},
		{baseURL: "http://192.0.2.10", wantErr: true},
		{baseURL: "http://localhost.example.test", wantErr: true},
	} {
		t.Run(test.baseURL, func(t *testing.T) {
			_, err := NewClient(test.baseURL, http.DefaultClient, Credentials{Username: testUsername, Password: testPassword})
			if test.wantErr && err == nil {
				t.Fatal("remote HTTP Jira URL was accepted")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("loopback/HTTPS URL rejected: %v", err)
			}
		})
	}
}

func TestClientRejectsSemanticallyInvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		call func(*Client) error
	}{
		{
			name: "issue null status",
			path: "/rest/api/3/issue/OMA-42",
			body: `{"key":"OMA-42","fields":{"summary":"summary","status":null}}`,
			call: func(client *Client) error {
				_, _, err := client.FetchIssue(context.Background(), "OMA-42")
				return err
			},
		},
		{
			name: "issue missing status category",
			path: "/rest/api/3/issue/OMA-42",
			body: `{"key":"OMA-42","fields":{"summary":"summary","status":{"id":"1","name":"Open"}}}`,
			call: func(client *Client) error {
				_, _, err := client.FetchIssue(context.Background(), "OMA-42")
				return err
			},
		},
		{
			name: "transition empty object",
			path: "/rest/api/3/issue/OMA-42/transitions",
			body: `{"transitions":[{}]}`,
			call: func(client *Client) error {
				_, err := client.Transitions(context.Background(), "OMA-42")
				return err
			},
		},
		{
			name: "transition missing destination category",
			path: "/rest/api/3/issue/OMA-42/transitions",
			body: `{"transitions":[{"id":"21","name":"Start","to":{"id":"3","name":"In Progress"}}]}`,
			call: func(client *Client) error {
				_, err := client.Transitions(context.Background(), "OMA-42")
				return err
			},
		},
		{
			name: "myself missing display name",
			path: "/rest/api/3/myself",
			body: `{"accountId":"acct-me"}`,
			call: func(client *Client) error {
				_, err := client.Myself(context.Background())
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Errorf("path = %s, want %s", r.URL.Path, test.path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, server.Client())
			err := test.call(client)
			assertSafeError(t, err, test.body)
		})
	}
}

func TestClientRejectsSemanticallyInvalidIssueFields(t *testing.T) {
	validStatus := `{"id":"1","name":"Open","statusCategory":{"key":"new"}}`
	tests := []struct {
		name   string
		fields string
	}{
		{name: "null summary", fields: `"summary":null,"status":` + validStatus},
		{name: "empty summary", fields: `"summary":"","status":` + validStatus},
		{name: "whitespace summary", fields: `"summary":"  \t ","status":` + validStatus},
		{name: "empty assignee object", fields: `"summary":"summary","status":` + validStatus + `,"assignee":{}`},
		{name: "assignee missing account ID", fields: `"summary":"summary","status":` + validStatus + `,"assignee":{"displayName":"Assigned User"}`},
		{name: "assignee whitespace account ID", fields: `"summary":"summary","status":` + validStatus + `,"assignee":{"accountId":"   "}`},
		{name: "empty description object", fields: `"summary":"summary","status":` + validStatus + `,"description":{}`},
		{name: "wrong description root", fields: `"summary":"summary","status":` + validStatus + `,"description":{"type":"paragraph","version":1,"content":[]}`},
		{name: "unsupported description version", fields: `"summary":"summary","status":` + validStatus + `,"description":{"type":"doc","version":2,"content":[]}`},
		{name: "missing description content", fields: `"summary":"summary","status":` + validStatus + `,"description":{"type":"doc","version":1}`},
		{name: "null description content", fields: `"summary":"summary","status":` + validStatus + `,"description":{"type":"doc","version":1,"content":null}`},
		{name: "malformed description content", fields: `"summary":"summary","status":` + validStatus + `,"description":{"type":"doc","version":1,"content":[1]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"key":"OMA-42","fields":{` + test.fields + `}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, server.Client())
			_, _, err := client.FetchIssue(context.Background(), "OMA-42")
			assertSafeError(t, err, body)
		})
	}
}

func TestClientAllowsNullDescriptionAndUnassignedOrIdentifiedAssignee(t *testing.T) {
	for _, assignee := range []string{`null`, `{"accountId":"acct-1"}`} {
		t.Run(assignee, func(t *testing.T) {
			body := `{"key":"OMA-42","fields":{"summary":"summary","status":{"id":"1","name":"Open","statusCategory":{"key":"new"}},"description":null,"assignee":` + assignee + `}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, server.Client())
			issue, _, err := client.FetchIssue(context.Background(), "OMA-42")
			if err != nil {
				t.Fatalf("FetchIssue: %v", err)
			}
			if issue.DescriptionText != "" {
				t.Fatalf("description = %q, want empty", issue.DescriptionText)
			}
			if assignee == `null` && issue.Assignee != nil {
				t.Fatalf("null assignee = %#v", issue.Assignee)
			}
			if assignee != `null` && (issue.Assignee == nil || issue.Assignee.AccountID != "acct-1") {
				t.Fatalf("identified assignee = %#v", issue.Assignee)
			}
		})
	}
}

func TestClientRendersADFBlockAndInlineDisplayText(t *testing.T) {
	description := `{"type":"doc","version":1,"content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"mention","attrs":{"text":"@Agent"}},{"type":"emoji","attrs":{"shortName":":wave:"}}]},` +
		`{"type":"codeBlock","content":[{"type":"text","text":"code()"}]},` +
		`{"type":"unknownWrapper","content":[{"type":"mention","text":"@Fallback"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"Done"}]}` +
		`]}`
	body := `{"key":"OMA-42","fields":{"summary":"summary","status":{"id":"1","name":"Open","statusCategory":{"key":"new"}},"description":` + description + `}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, server.Client())
	issue, _, err := client.FetchIssue(context.Background(), "OMA-42")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	want := "Hello @Agent:wave:\ncode()\n@FallbackDone"
	if issue.DescriptionText != want {
		t.Fatalf("description = %q, want %q", issue.DescriptionText, want)
	}
}

func TestCredentialsFromNetrcRejectsMalformedWithoutPanic(t *testing.T) {
	for _, content := range []string{
		"machine",
		"machine ",
		"machine jira.example.test login",
	} {
		t.Run(strings.ReplaceAll(content, " ", "_"), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "netrc")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			var credentials Credentials
			var err error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("CredentialsFromNetrc panicked: %v", recovered)
					}
				}()
				credentials, err = CredentialsFromNetrc(path, "jira.example.test")
			}()
			if err == nil {
				t.Fatalf("malformed netrc returned credentials: %#v", credentials)
			}
			assertSafeError(t, err, content)
		})
	}
}

func newTestClient(t *testing.T, baseURL string, httpClient *http.Client) *Client {
	t.Helper()
	client, err := NewClient(baseURL, httpClient, Credentials{Username: testUsername, Password: testPassword})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func assertBasicAuth(t *testing.T, r *http.Request) {
	t.Helper()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(testUsername+":"+testPassword))
	if got := r.Header.Get("Authorization"); got != want {
		t.Error("Authorization header did not contain the expected reserved test credentials")
	}
}

func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("got invalid JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("want invalid JSON: %v", err)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("JSON = %s, want %s", gotJSON, wantJSON)
	}
}

func assertSafeError(t *testing.T, err error, forbiddenBody string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	for _, forbidden := range []string{testUsername, testPassword, "Authorization", forbiddenBody} {
		if forbidden != "" && strings.Contains(message, forbidden) {
			t.Fatalf("error leaked forbidden content: %q", message)
		}
	}
	if len(message) > 512 {
		t.Fatalf("error length = %d, want <= 512", len(message))
	}
}
