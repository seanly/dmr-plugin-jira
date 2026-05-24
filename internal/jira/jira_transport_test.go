package jira

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTransientTransportErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"http post eof", errors.New(`Post "https://jira.example/rest/api/2/search": EOF`), true},
		{"reset peer", errors.New(`read tcp 10.0.0.1:1->203.0.113.9:443: read: connection reset by peer`), true},
		{"broken pipe", errors.New(`write tcp ... broken pipe`), true},
		{"timeout", fakeNetTimeoutErr{}, true},
		{"http 500", errors.New(`HTTP internal error`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := transientTransportErr(tc.err); got != tc.want {
				t.Fatalf("transientTransportErr(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeNetTimeoutErr struct{}

func (fakeNetTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeNetTimeoutErr) Timeout() bool   { return true }
func (fakeNetTimeoutErr) Temporary() bool { return true }

// flakyRoundTripTransport fails the first attempts with synthetic reset, then succeeds.
type flakyRoundTripTransport struct {
	attempt int
	okBody  string
}

func (f *flakyRoundTripTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.attempt++
	if f.attempt < 3 {
		return nil, errors.New(`read tcp 127.0.0.1:1->198.18.0.9:443: read: connection reset by peer`)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(f.okBody)),
	}, nil
}

func TestDoJSON_RetriesOnReset(t *testing.T) {
	rt := &flakyRoundTripTransport{okBody: `{"issues":[]}`}
	c := &JiraClient{
		baseURL: "https://stub.invalid",
		user:    "u",
		password: "p",
		httpClient: &http.Client{
			Transport: rt,
		},
	}
	payload := map[string]any{"jql": "assignee = me", "maxResults": 1}
	data, code, err := c.doJSON(http.MethodPost, "/rest/api/2/search", nil, payload)
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	if string(data) != `{"issues":[]}` {
		t.Fatalf("body %s", data)
	}
	if rt.attempt != 3 {
		t.Fatalf("expected 3 transports, got %d", rt.attempt)
	}
}
