package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
