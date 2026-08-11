package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
)

func TestLoginLang(t *testing.T) {
	cases := map[string]string{
		"":                          "en",
		"en-US,en;q=0.9":            "en",
		"tr-TR,tr;q=0.9,en;q=0.8":   "tr",
		"tr":                        "tr",
		"TR-tr":                     "tr",
		"de-DE,de;q=0.9":            "en", // no German catalogue yet -> English
		"de-DE,de;q=0.9,tr;q=0.5":   "tr", // but a known language further down wins
		"fr-FR,fr;q=0.9,en-GB;q=.8": "en",
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/login", nil)
		req.Header.Set("Accept-Language", header)
		if got := loginLang(req); got != want {
			t.Errorf("Accept-Language %q -> %q, wanted %q", header, got, want)
		}
	}
}

// TestLoginTextCatalogueComplete: a half-translated login page would ship an
// empty button rather than fall back, because the template reads fields
// directly.
func TestLoginTextCatalogueComplete(t *testing.T) {
	for lang, s := range loginText {
		if s.Title == "" || s.Intro == "" || s.Placeholder == "" || s.Submit == "" ||
			s.BadForm == "" || s.Disabled == "" || s.WrongToken == "" {
			t.Errorf("%q: login catalogue has an empty field: %+v", lang, s)
		}
	}
	if _, ok := loginText["en"]; !ok {
		t.Error("English is the fallback and must exist")
	}
}

// TestErrorResponseCarriesCode is the contract the browser relies on to show a
// message in its own language: a stable code plus the substitution arguments.
func TestErrorResponseCarriesCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest,
		apperr.New(apperr.CodeProjectDuplicate, "this folder is already tracked: %s", "my-app"))

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		Args  []any  `json:"args"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != apperr.CodeProjectDuplicate {
		t.Errorf("code = %q", body.Code)
	}
	if len(body.Args) != 1 || body.Args[0] != "my-app" {
		t.Errorf("args = %v; the UI cannot rebuild the sentence without them", body.Args)
	}
	if !strings.Contains(body.Error, "my-app") {
		t.Errorf("English fallback lost the argument: %q", body.Error)
	}
}

// TestPlainErrorStillReported: not every failure has a code, and those must
// still reach the user rather than vanishing.
func TestPlainErrorStillReported(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, errPlain("disk on fire"))

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "disk on fire" {
		t.Errorf("error = %v", body["error"])
	}
	if _, has := body["code"]; has {
		t.Error("a plain error should not invent a code")
	}
}

type errPlain string

func (e errPlain) Error() string { return string(e) }

// TestAPIErrorsAreEnglish guards the default language of the API surface.
// A Turkish sentence here would be unreadable for anyone whose UI has no
// translation for that code.
func TestAPIErrorsAreEnglish(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})

	req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"path":""}`))
	req.Host = "localhost"
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)

	if body.Code == "" {
		t.Error("no code on a validation failure")
	}
	if strings.ContainsAny(body.Error, "ıİğĞşŞçÇöÖüÜ") {
		t.Errorf("API error is not English: %q", body.Error)
	}
}
