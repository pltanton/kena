package web

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
)

type connectView struct {
	LoggedIn       bool
	LoginURL       string
	MCPURL         string
	ClaudeURL      template.URL
	CursorURL      template.URL
	VSCodeURL      template.URL
	PromptPhoto    string
	PromptMistakes string
	NewToken       *newToken
	Error          string
}

func (h *handler) renderConnect(w http.ResponseWriter, r *http.Request, v connectView) {
	v.MCPURL = h.publicURL + "/mcp"
	v.ClaudeURL = template.URL("https://claude.ai/customize/connectors?" + url.Values{
		"modal":         {"add-custom-connector"},
		"connectorName": {"kena"},
		"connectorUrl":  {v.MCPURL},
	}.Encode())
	cursorConfig, _ := json.Marshal(struct {
		URL string `json:"url"`
	}{v.MCPURL})
	v.CursorURL = template.URL("cursor://anysphere.cursor-deeplink/mcp/install?" + url.Values{
		"name":   {"kena"},
		"config": {base64.StdEncoding.EncodeToString(cursorConfig)},
	}.Encode())
	vscodeConfig, _ := json.Marshal(struct {
		Name string `json:"name"`
		Type string `json:"type"`
		URL  string `json:"url"`
	}{"kena", "http", v.MCPURL})
	v.VSCodeURL = template.URL("vscode:mcp/install?" + url.QueryEscape(string(vscodeConfig)))
	v.PromptPhoto, v.PromptMistakes = tr(r, "prompt.photo"), tr(r, askPrompt)
	if _, ok := currentUser(r); ok {
		v.LoggedIn = true
	} else {
		v.LoginURL = loginURL("/connect")
	}
	h.page(w, r, http.StatusOK, "connect", v)
}

func (h *handler) connect(w http.ResponseWriter, r *http.Request) {
	h.renderConnect(w, r, connectView{})
}
