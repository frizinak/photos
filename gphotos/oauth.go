package gphotos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skratchdot/open-golang/open"
)

const (
	EndpointAuth  = "https://accounts.google.com/o/oauth2/v2/auth"
	EndpointToken = "https://oauth2.googleapis.com/token"
)

func (g *GPhotos) genCodeVerifier() ([]byte, error) {
	v, err := codeVerifier(g.state.codeVerifierLength)
	if err != nil {
		return nil, err
	}
	g.state.codeVerifier = v
	return g.state.codeVerifier, nil
}

func (g *GPhotos) codeVerifier() ([]byte, error) {
	if len(g.state.codeVerifier) != g.state.codeVerifierLength {
		return nil, errors.New("no code verifier generated")
	}
	return g.state.codeVerifier, nil
}

func (g *GPhotos) Auth(scopes []string) (string, error) {
	codeVerifier, err := g.genCodeVerifier()
	if err != nil {
		return "", err
	}

	u, err := url.Parse(EndpointAuth)
	if err != nil {
		return "", err
	}

	loopback, err := loopbackAddr()
	if err != nil {
		return "", err
	}

	var codeResponse string
	var wg sync.WaitGroup
	wg.Add(1)
	var gerr error
	go func() {
		mux := http.NewServeMux()
		server := http.Server{Addr: loopback, Handler: mux}
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			codeResponse = r.URL.Query().Get("code")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success, you can close this tab now."))
			go server.Shutdown(nil)
		})

		if err := server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				gerr = err
			}
		}
		wg.Done()
	}()

	g.state.redirect = "http://" + loopback

	q := u.Query()
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("response_type", "code")
	q.Set("code_challenge", string(codeVerifier))
	q.Set("code_challenge_method", "plain")
	q.Set("access_type", "offline")
	q.Set("redirect_uri", g.state.redirect)
	q.Set("client_id", g.id)
	u.RawQuery = q.Encode()

	g.l.Printf("Opening\n\n%s\n\nin your default browser, please log in and grant access", u.String())
	open.Run(u.String())

	wg.Wait()
	if gerr != nil {
		return codeResponse, gerr
	}

	if codeResponse == "" {
		return codeResponse, errors.New("no code received")
	}

	return codeResponse, nil
}

type Token struct {
	Expires    time.Time
	Access     string `json:"access_token"`
	Refresh    string `json:"refresh_token"`
	ExpiresIn  int    `json:"expires_in"`
	Scope      string `json:"scope"`
	Error      string `json:"error,omitempty"`
	ErrorDescr string `json:"error_description,omitempty"`
}

func (g *GPhotos) Token(code string) error {
	codeVerifier, err := g.codeVerifier()
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("client_id", g.id)
	q.Set("client_secret", g.secret)
	q.Set("code", code)
	q.Set("code_verifier", string(codeVerifier))
	q.Set("grant_type", "authorization_code")
	q.Set("redirect_uri", g.state.redirect)

	req, err := http.NewRequest("POST", EndpointToken, strings.NewReader(q.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return g.token(req)
}

func (g *GPhotos) Refresh() error {
	q := url.Values{}
	q.Set("client_id", g.id)
	q.Set("client_secret", g.secret)
	q.Set("refresh_token", g.t.Refresh)
	q.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", EndpointToken, strings.NewReader(q.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return g.token(req)
}

func (g *GPhotos) token(req *http.Request) error {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	t, err := g.decode(res.Body)
	if err != nil {
		return err
	}

	g.t.Access = t.Access
	g.t.Refresh = t.Refresh
	g.t.ExpiresIn = t.ExpiresIn
	g.t.Scope = t.Scope

	g.t.Expires = time.Now().Add(time.Second * time.Duration(t.ExpiresIn))
	return nil
}

func (g *GPhotos) AuthHeader(h http.Header) error {
	if err := g.Authenticate(false); err != nil {
		return err
	}

	h.Set("Authorization", fmt.Sprintf("Bearer %s", g.t.Access))
	return nil
}

func (g *GPhotos) decode(r io.ReadCloser) (Token, error) {
	t := Token{}
	err := json.NewDecoder(r).Decode(&t)
	r.Close()
	if err == nil && t.Error != "" {
		return t, fmt.Errorf("%s: %s", t.Error, t.ErrorDescr)
	}

	return t, err
}

func (g *GPhotos) Authenticate(interactive bool) error {
	isNew, err := g.ensureToken(interactive)
	if err != nil {
		return err
	}
	if !isNew {
		return nil
	}

	f, err := os.OpenFile(g.cache, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(g.t)
}

func (g *GPhotos) ensureToken(interactive bool) (bool, error) {
	g.sem.Lock()
	defer g.sem.Unlock()
	if g.t.Access != "" && g.t.Expires.Add(-5*time.Minute).After(time.Now()) {
		return false, nil
	}

	f, err := os.Open(g.cache)
	if err == nil {
		g.t, _ = g.decode(f)
	}

	if g.t.Access != "" && g.t.Expires.Add(-5*time.Minute).After(time.Now()) {
		return false, nil
	}

	if g.t.Refresh != "" {
		return true, g.Refresh()
	}

	if !interactive {
		return false, errors.New("No refresh token available, requires user interaction")
	}

	code, err := g.Auth(
		[]string{
			"https://www.googleapis.com/auth/photoslibrary.appendonly",
			"https://www.googleapis.com/auth/photoslibrary.readonly",
		},
	)

	if err != nil {
		return true, err
	}

	return true, g.Token(code)
}
