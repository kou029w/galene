package webserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jech/galene/diskwriter"
	"github.com/jech/galene/group"
	"github.com/jech/galene/token"
)

var setupOnce sync.Once

func setup() {
	setupOnce.Do(func() {
		Insecure = true
		Static = http.Dir(os.TempDir())
		err := Serve("localhost:1234", "")
		if err != nil {
			panic("could not start server")
		}
	})
}

func setupTest(dir, datadir string) error {
	setup()

	group.Directory = dir
	group.DataDirectory = datadir
	config := `{
    "users": {
        "admin": {
            "password": "secret",
            "permissions": "admin"
        },
        "op": {
            "password": "secret",
            "permissions": "op"
        }
    }
}`
	err := os.WriteFile(filepath.Join(datadir, "config.json"), []byte(config), 0600)
	if err != nil {
		return err
	}
	group.ResetConfiguration()

	token.SetStatefulFilename(filepath.Join(datadir, "tokens.jsonl"))
	token.ResetTokens()

	return nil
}

func query(method, url, auth, body string) (int, []byte, error) {
	req, err := http.NewRequest(method, "http://localhost:1234"+url,
		bytes.NewReader([]byte(body)))
	if err != nil {
		return 0, nil, err
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	return res.StatusCode, resBody, err
}

func queryJSON[T any](method, url, auth, body string) (int, *T, error) {
	status, resBody, err := query(method, url, auth, body)
	if err != nil {
		return status, nil, err
	}
	var res T
	err = json.Unmarshal(resBody, &res)
	if err != nil {
		return status, nil, err
	}
	return status, &res, nil
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	code, bytes, err := query("GET", "/galene-api/v0/.stats", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Stats: code is %v", code)
	}
	if len(bytes) == 0 {
		t.Errorf("Stats: body is empty")
	}
}

func TestUsers(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	code, bytes, err := query("GET", "/galene-api/v0/.users", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Errorf("Users: unauthenticated access got %v", code)
	}

	code, users, err := queryJSON[[]group.UserDescription](
		"GET", "/galene-api/v0/.users", "Basic YWRtaW46c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Users: got %v", code)
	}
	if len(*users) != 2 {
		t.Errorf("Users: got %v", *users)
	}
}

func TestToken(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	code, bytes, err := query("GET", "/galene-api/v0/.tokens/", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Errorf("Tokens: unauthenticated access got %v", code)
	}

	code, bytes, err = query("GET", "/galene-api/v0/.tokens/",
		"Basic b3A6c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Errorf("Tokens: non-admin access got %v", code)
	}

	code, tokens, err := queryJSON[[]tokenEntry](
		"GET", "/galene-api/v0/.tokens/",
		"Basic YWRtaW46c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Tokens: got %v", code)
	}
	if len(*tokens) != 0 {
		t.Errorf("Tokens: got %v", *tokens)
	}

	var tok tokenEntry
	code, tok2, err := queryJSON[tokenEntry](
		"POST", "/galene-api/v0/.tokens/",
		"Basic YWRtaW46c2VjcmV0",
		`{"group": "public", "permissions": "present"}`,
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 201 {
		t.Errorf("Tokens: got %v", code)
	}
	tok = *tok2

	code, tokens, err = queryJSON[[]tokenEntry](
		"GET", "/galene-api/v0/.tokens/",
		"Basic YWRtaW46c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Tokens: got %v", code)
	}
	if len(*tokens) != 1 || (*tokens)[0].Id != tok.Id {
		t.Errorf("Tokens: got %v, expected %v", *tokens, tok)
	}

	code, tok2, err = queryJSON[tokenEntry](
		"GET", "/galene-api/v0/.tokens/"+tok.Id,
		"Basic YWRtaW46c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Tokens: got %v", code)
	}
	if tok2.Id != tok.Id || tok2.Group != tok.Group {
		t.Errorf("Tokens: got %v, expected %v", tok2, tok)
	}

	code, bytes, err = query("DELETE", "/galene-api/v0/.tokens/"+tok.Id,
		"Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 204 {
		t.Errorf("Tokens: got %v", code)
	}

	code, bytes, err = query("GET", "/galene-api/v0/.tokens/"+tok.Id,
		"Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 404 {
		t.Errorf("Tokens: got %v", code)
	}
}

func TestGroups(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	g, err := group.Add("test", nil)
	if err != nil {
		t.Fatalf("group.Add: %v", err)
	}
	desc := g.Description()
	desc.Public = true
	err = group.SetDescription("test", desc)
	if err != nil {
		t.Fatalf("group.SetDescription: %v", err)
	}

	code, groups, err := queryJSON[[]groupEntry](
		"GET", "/galene-api/v0/.groups/", "", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Groups: got %v", code)
	}
	if len(*groups) != 1 || (*groups)[0].Name != "test" {
		t.Errorf("Groups: got %v", *groups)
	}

	code, group, err := queryJSON[groupEntry](
		"GET", "/galene-api/v0/.groups/test", "", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Groups: got %v", code)
	}
	if group.Name != "test" {
		t.Errorf("Groups: got %v", group)
	}

	code, bytes, err := query("DELETE", "/galene-api/v0/.groups/test", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Errorf("Groups: unauthenticated access got %v", code)
	}

	code, bytes, err = query("DELETE", "/galene-api/v0/.groups/test",
		"Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 204 {
		t.Errorf("Groups: got %v", code)
	}

	code, bytes, err = query("GET", "/galene-api/v0/.groups/test", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 404 {
		t.Errorf("Groups: got %v", code)
	}
}

func TestGalenectl(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	var conf group.GalenectlConfig
	code, confp, err := queryJSON[group.GalenectlConfig](
		"GET", "/galene-api/v0/.galenectl", "Basic YWRtaW46c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Fatalf("Galenectl: got %v", code)
	}
	conf = *confp

	if conf.URL != "http://localhost:1234/galene-api/v0/" {
		t.Errorf("Galenectl: url is %v", conf.URL)
	}
	if conf.Username != "admin" || conf.Password != "secret" {
		t.Errorf("Galenectl: user is %v, pass is %v",
			conf.Username, conf.Password)
	}

	code, confp, err = queryJSON[group.GalenectlConfig](
		"GET", "/galene-api/v0/.galenectl", "Basic b3A6c2VjcmV0", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Fatalf("Galenectl: non-admin got %v", code)
	}

	code, confp, err = queryJSON[group.GalenectlConfig](
		"GET", "/galene-api/v0/.galenectl", "", "",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Fatalf("Galenectl: unauthenticated got %v", code)
	}
}

func TestRecordings(t *testing.T) {
	dir := t.TempDir()
	datadir := t.TempDir()
	err := setupTest(dir, datadir)
	if err != nil {
		t.Fatalf("setupTest: %v", err)
	}

	code, bytes, err := query("GET", "/recordings/public", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 308 {
		t.Errorf("Recordings: got %v, expected 308", code)
	}

	code, bytes, err = query("GET", "/recordings/public/", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 401 {
		t.Errorf("Recordings: got %v, expected 401", code)
	}

	code, bytes, err = query("GET", "/recordings/public/", "Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 404 {
		t.Errorf("Recordings: got %v, expected 404", code)
	}

	recdir := filepath.Join(diskwriter.Directory, "public")
	err = os.MkdirAll(recdir, 0700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	code, bytes, err = query("GET", "/recordings/public/", "Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Recordings: got %v, expected 200", code)
	}

	code, bytes, err = query("GET", "/recordings/public/foo", "Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 404 {
		t.Errorf("Recordings: got %v, expected 404", code)
	}

	err = os.WriteFile(filepath.Join(recdir, "foo"), []byte("123"), 0600)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, bytes, err = query("GET", "/recordings/public/foo", "Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 200 {
		t.Errorf("Recordings: got %v, expected 200", code)
	}
	if string(bytes) != "123" {
		t.Errorf("Recordings: got %v, expected 123", bytes)
	}

	code, bytes, err = query("POST", "/recordings/public/?q=delete&filename=foo", "Basic YWRtaW46c2VjcmV0", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if code != 303 {
		t.Errorf("Recordings: got %v, expected 303", code)
	}

	_, err = os.Stat(filepath.Join(recdir, "foo"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Recordings: foo was not deleted: %v", err)
	}
}
