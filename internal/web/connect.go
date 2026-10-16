package web

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
)

const connectPromptPhoto = "Here is a photo of an exercise from my Greek book. Make 100 more like it and upload them to kena."

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

func connectLinks(publicURL string) (mcpURL, claudeURL, cursorURL, vscodeURL string) {
	mcpURL = publicURL + "/mcp"

	claudeURL = "https://claude.ai/customize/connectors?" + url.Values{
		"modal":         {"add-custom-connector"},
		"connectorName": {"kena"},
		"connectorUrl":  {mcpURL},
	}.Encode()

	cursorConfig, _ := json.Marshal(struct {
		URL string `json:"url"`
	}{mcpURL})
	cursorURL = "cursor://anysphere.cursor-deeplink/mcp/install?" + url.Values{
		"name":   {"kena"},
		"config": {base64.StdEncoding.EncodeToString(cursorConfig)},
	}.Encode()

	vscodeConfig, _ := json.Marshal(struct {
		Name string `json:"name"`
		Type string `json:"type"`
		URL  string `json:"url"`
	}{"kena", "http", mcpURL})
	vscodeURL = "vscode:mcp/install?" + url.QueryEscape(string(vscodeConfig))

	return mcpURL, claudeURL, cursorURL, vscodeURL
}

func (h *handler) renderConnect(w http.ResponseWriter, r *http.Request, status int, v connectView) {
	mcpURL, claudeURL, cursorURL, vscodeURL := connectLinks(h.publicURL)
	v.MCPURL, v.ClaudeURL, v.CursorURL, v.VSCodeURL = mcpURL, template.URL(claudeURL), template.URL(cursorURL), template.URL(vscodeURL)
	v.PromptPhoto, v.PromptMistakes = connectPromptPhoto, askPrompt
	if _, ok := currentUser(r); ok {
		v.LoggedIn = true
	} else {
		v.LoginURL = "/login?" + url.Values{"next": {"/connect"}}.Encode()
	}
	h.page(w, r, status, "connect", v)
}

func (h *handler) connect(w http.ResponseWriter, r *http.Request) {
	h.renderConnect(w, r, http.StatusOK, connectView{})
}
