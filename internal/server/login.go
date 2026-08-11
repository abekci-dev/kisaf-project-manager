package server

import (
	"html/template"
	"net/http"
	"strings"
	"time"
)

// The login page only ever appears for requests coming from another machine.
//
// It is deliberately a plain server-rendered form, and it is the one screen the
// browser's own catalogue cannot translate: it has to work before any
// JavaScript loads. So it picks its language from Accept-Language and ships
// both strings inline.
var loginTmpl = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>kisaf · {{.T.Title}}</title>
<link rel="icon" href="/icons/favicon-32.png" sizes="32x32">
<style>
  :root { color-scheme: dark; }
  body { margin:0; min-height:100vh; display:grid; place-items:center;
         background:#0f1115; color:#e6e8ec;
         font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; }
  form { width:min(360px,90vw); background:#171a21; padding:28px;
         border:1px solid #262b36; border-radius:14px; }
  h1 { margin:0 0 4px; font-size:20px; }
  p  { margin:0 0 20px; color:#8b93a7; font-size:13px; }
  input { width:100%; box-sizing:border-box; padding:11px 13px; margin-bottom:14px;
          background:#0f1115; color:#e6e8ec; border:1px solid #2d3340;
          border-radius:9px; font-size:14px; }
  input:focus { outline:none; border-color:#4c8dff; }
  button { width:100%; padding:11px; background:#4c8dff; color:#fff; border:0;
           border-radius:9px; font-size:14px; font-weight:600; cursor:pointer; }
  .err { background:#3b1d22; border:1px solid #6b2b34; color:#ffb4be;
         padding:9px 12px; border-radius:9px; margin-bottom:14px; font-size:13px; }
</style>
</head><body>
<form method="post" action="/login">
  <h1>kisaf</h1>
  <p>{{.T.Intro}}</p>
  {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
  <input type="hidden" name="next" value="{{.Next}}">
  <input type="password" name="token" placeholder="{{.T.Placeholder}}" autofocus required>
  <button type="submit">{{.T.Submit}}</button>
</form>
</body></html>`))

// loginStrings is the small catalogue this page needs.
type loginStrings struct {
	Title       string
	Intro       string
	Placeholder string
	Submit      string
	BadForm     string
	Disabled    string
	WrongToken  string
}

var loginText = map[string]loginStrings{
	"en": {
		Title:       "Sign in",
		Intro:       "This device is connecting remotely. Enter the access key to continue.",
		Placeholder: "Access key",
		Submit:      "Sign in",
		BadForm:     "Could not read the form.",
		Disabled:    "Remote access is disabled.",
		WrongToken:  "Wrong key.",
	},
	"tr": {
		Title:       "Giriş",
		Intro:       "Bu cihaz uzaktan bağlanıyor. Devam etmek için erişim anahtarını girin.",
		Placeholder: "Erişim anahtarı",
		Submit:      "Giriş yap",
		BadForm:     "Form okunamadı.",
		Disabled:    "Uzaktan erişim kapalı.",
		WrongToken:  "Anahtar hatalı.",
	},
}

// loginLang picks a language from Accept-Language, defaulting to English.
//
// A full RFC 4647 match would be overkill for a two-language page: the tags we
// care about are only ever a prefix away.
func loginLang(r *http.Request) string {
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if tag == "tr" || strings.HasPrefix(tag, "tr-") {
			return "tr"
		}
		if tag == "en" || strings.HasPrefix(tag, "en-") {
			return "en"
		}
	}
	return "en"
}

type loginData struct {
	Lang  string
	T     loginStrings
	Error string
	Next  string
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if isLoopback(r.RemoteAddr) || s.tokenOK(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.renderLogin(w, r, http.StatusOK, "", r.URL.Query().Get("next"))
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	text := loginText[loginLang(r)]

	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, r, http.StatusBadRequest, text.BadForm, "/")
		return
	}
	next := sanitizeNext(r.FormValue("next"))

	if s.cfg.Token == "" {
		s.renderLogin(w, r, http.StatusForbidden, text.Disabled, next)
		return
	}
	if !constantEqual(r.FormValue("token"), s.cfg.Token) {
		// A small delay takes brute forcing over a LAN off the table without
		// any state to keep.
		time.Sleep(500 * time.Millisecond)
		s.renderLogin(w, r, http.StatusUnauthorized, text.WrongToken, next)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookie,
		Value:    s.cfg.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60,
	})
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errMsg, next string) {
	lang := loginLang(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = loginTmpl.Execute(w, loginData{
		Lang:  lang,
		T:     loginText[lang],
		Error: errMsg,
		Next:  sanitizeNext(next),
	})
}

// sanitizeNext keeps the post-login redirect on this site: "//evil.com" is a
// protocol-relative URL and would otherwise take the user off the box.
func sanitizeNext(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}
