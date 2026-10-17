package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rsc.io/qr"

	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

const loginCodeTTL = 5 * time.Minute

func qrSVG(text, label string) (template.HTML, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", fmt.Errorf("encoding QR: %w", err)
	}
	const quiet = 4
	n := code.Size + 2*quiet
	var d strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&d, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return template.HTML(fmt.Sprintf(`<svg class="qr" viewBox="0 0 %d %d" role="img" aria-label="%s" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="%s"/></svg>`, n, n, template.HTMLEscapeString(label), n, n, d.String())), nil
}

func (h *handler) createLoginQR(w http.ResponseWriter, r *http.Request, u store.User) {
	code, err := h.st.CreateLoginCode(r.Context(), u.ID, loginCodeTTL)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	svg, err := qrSVG(h.publicURL+"/login/qr/"+url.PathEscape(code), tr(r, "qr.label"))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.renderAccount(w, r, u, accountView{QR: svg})
}

type qrLoginView struct {
	Username string
	Current  string
	Action   string
}

func (h *handler) codeUser(w http.ResponseWriter, r *http.Request, lookup func(context.Context, string) (store.User, error)) (store.User, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	u, err := lookup(r.Context(), r.PathValue("secret"))
	if errors.Is(err, store.ErrNotFound) {
		h.page(w, r, http.StatusGone, "qr_login", qrLoginView{})
		return u, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return u, false
	}
	return u, true
}

func (h *handler) qrLoginPage(w http.ResponseWriter, r *http.Request) {
	u, ok := h.codeUser(w, r, h.st.PeekLoginCode)
	if !ok {
		return
	}
	v := qrLoginView{Username: u.Username, Action: "/login/qr/" + url.PathEscape(r.PathValue("secret"))}
	if cur, ok := currentUser(r); ok {
		v.Current = cur.Username
	}
	h.page(w, r, http.StatusOK, "qr_login", v)
}

func (h *handler) qrLogin(w http.ResponseWriter, r *http.Request) {
	u, ok := h.codeUser(w, r, h.st.UseLoginCode)
	if !ok {
		return
	}
	if err := session.Start(w, r, h.st, u.ID); err != nil {
		h.serverError(w, r, err)
		return
	}
	h.loggedIn(w, r, u, "/")
}
