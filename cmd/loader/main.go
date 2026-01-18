// Package main provides a CLI that loads MediaMTX YAML config paths into a running MediaMTX via HTTP API.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type mode string

const (
	modeAdd     mode = "add"
	modePatch   mode = "patch"
	modeReplace mode = "replace"
)

func mustPrettyJSON(v any) string {
	byts, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		byts, _ = json.Marshal(v)
	}
	return string(byts)
}

func convertKeys(i any) (any, error) {
	switch x := i.(type) {
	case map[any]any:
		m2 := map[string]any{}
		for k, v := range x {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("non-string YAML key is not supported (%v)", k)
			}
			c, err := convertKeys(v)
			if err != nil {
				return nil, err
			}
			m2[ks] = c
		}
		return m2, nil

	case []any:
		a2 := make([]any, len(x))
		for i, v := range x {
			c, err := convertKeys(v)
			if err != nil {
				return nil, err
			}
			a2[i] = c
		}
		return a2, nil
	}

	return i, nil
}

func encodePathName(name string) string {
	// keep "/" unescaped, escape everything else that might break the URL path.
	segs := strings.Split(name, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func authHeader(user, pass string) string {
	raw := []byte(user + ":" + pass)
	return "Basic " + base64.StdEncoding.EncodeToString(raw)
}

func httpJSON(
	c *http.Client,
	method string,
	u string,
	body any,
	auth string,
) (int, string, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	res, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()

	byts, _ := io.ReadAll(res.Body)
	return res.StatusCode, strings.TrimSpace(string(byts)), nil
}

func joinURL(base string, p string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(p, "/")
}

func usage() {
	fmt.Fprintln(os.Stderr, "Load paths from a MediaMTX YAML config into a running MediaMTX via HTTP API.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Example:")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/loader -config mediamtx2.yml -api http://localhost:9997 -mode add")
	fmt.Fprintln(os.Stderr)
	flag.PrintDefaults()
}

func main() {
	var (
		config  = flag.String("config", "", "YAML file with a top-level 'paths:' mapping (e.g. mediamtx2.yml)")
		apiBase = flag.String("api", "http://localhost:9997", "MediaMTX API base URL")
		modeS   = flag.String("mode", string(modeAdd), "One of: add, patch, replace")
		user    = flag.String("user", "", "API basic-auth username (optional)")
		pass    = flag.String("pass", "", "API basic-auth password (optional)")
		timeout = flag.Duration("timeout", 5*time.Second, "HTTP timeout")
		dryRun  = flag.Bool("dry-run", false, "Print actions, do not call the API")
	)
	flag.Usage = usage
	flag.Parse()

	if *config == "" {
		usage()
		os.Exit(2)
	}

	m := mode(*modeS)
	if m != modeAdd && m != modePatch && m != modeReplace {
		fmt.Fprintf(os.Stderr, "invalid -mode %q\n", *modeS)
		os.Exit(2)
	}

	buf, err := os.ReadFile(*config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}

	var temp any
	if yamlErr := yaml.UnmarshalStrict(buf, &temp); yamlErr != nil {
		fmt.Fprintf(os.Stderr, "parse yaml: %v\n", yamlErr)
		os.Exit(1)
	}

	temp, err = convertKeys(temp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse yaml: %v\n", err)
		os.Exit(1)
	}

	root, ok := temp.(map[string]any)
	if !ok {
		fmt.Fprintln(os.Stderr, "config root must be a mapping")
		os.Exit(2)
	}

	pathsAny, ok := root["paths"]
	if !ok {
		fmt.Fprintln(os.Stderr, "config does not contain a top-level 'paths:' mapping")
		os.Exit(2)
	}

	paths, ok := pathsAny.(map[string]any)
	if !ok {
		fmt.Fprintln(os.Stderr, "'paths' must be a mapping")
		os.Exit(2)
	}

	var auth string
	if *user != "" {
		auth = authHeader(*user, *pass)
	}

	client := &http.Client{Timeout: *timeout}

	added := 0
	updated := 0
	skipped := 0
	failed := 0

	printRequest := func(action string, name string, method string, u string, body map[string]any) {
		fmt.Printf("%s %s\n", action, name)
		fmt.Printf("  request.method: %s\n", method)
		fmt.Printf("  request.url: %s\n", u)
		fmt.Printf("  request.body: %s\n", mustPrettyJSON(body))
	}

	printResponse := func(status int, msg string) {
		fmt.Printf("  response.status: %d\n", status)
		if msg == "" {
			fmt.Printf("  response.body: (empty)\n")
		} else {
			fmt.Printf("  response.body: %s\n", msg)
		}
	}

	for name, bodyAny := range paths {
		bodyMap := map[string]any{}
		if bodyAny != nil {
			bm, okBody := bodyAny.(map[string]any)
			if !okBody {
				fmt.Fprintf(os.Stderr, "%s: skipping (path config must be a mapping)\n", name)
				skipped++
				continue
			}
			bodyMap = bm
		}

		encName := encodePathName(name)
		getURL := joinURL(*apiBase, "/v3/config/paths/get/"+encName)
		addURL := joinURL(*apiBase, "/v3/config/paths/add/"+encName)
		patchURL := joinURL(*apiBase, "/v3/config/paths/patch/"+encName)
		replaceURL := joinURL(*apiBase, "/v3/config/paths/replace/"+encName)

		if *dryRun {
			action := "ADD"
			method := http.MethodPost
			u := addURL

			switch m {
			case modePatch:
				action = "PATCH"
				method = http.MethodPatch
				u = patchURL

			case modeReplace:
				action = "REPLACE"
				method = http.MethodPost
				u = replaceURL
			}

			printRequest(action, name, method, u, bodyMap)
			fmt.Printf("  dry-run: true\n")
			added++
			continue
		}

		st, msg, httpErr := httpJSON(client, http.MethodGet, getURL, nil, auth)
		if httpErr != nil {
			fmt.Fprintf(os.Stderr, "%s: GET error: %v\n", name, httpErr)
			failed++
			continue
		}
		exists := st == http.StatusOK
		if st != http.StatusOK && st != http.StatusNotFound {
			fmt.Fprintf(os.Stderr, "%s: GET failed (status=%d) %s\n", name, st, msg)
			failed++
			continue
		}

		if !exists {
			printRequest("ADD", name, http.MethodPost, addURL, bodyMap)
			st, msg, err = httpJSON(client, http.MethodPost, addURL, bodyMap, auth)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: add error: %v\n", name, err)
				failed++
				continue
			}
			if st != http.StatusOK {
				printResponse(st, msg)
				fmt.Fprintf(os.Stderr, "%s: add failed (status=%d) %s\n", name, st, msg)
				failed++
				continue
			}
			printResponse(st, msg)
			added++
			continue
		}

		// exists
		if m == modeAdd {
			skipped++
			continue
		}

		if m == modePatch {
			printRequest("PATCH", name, http.MethodPatch, patchURL, bodyMap)
			st, msg, err = httpJSON(client, http.MethodPatch, patchURL, bodyMap, auth)
		} else {
			printRequest("REPLACE", name, http.MethodPost, replaceURL, bodyMap)
			st, msg, err = httpJSON(client, http.MethodPost, replaceURL, bodyMap, auth)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s error: %v\n", name, m, err)
			failed++
			continue
		}
		if st != http.StatusOK {
			printResponse(st, msg)
			fmt.Fprintf(os.Stderr, "%s: %s failed (status=%d) %s\n", name, m, st, msg)
			failed++
			continue
		}
		printResponse(st, msg)
		updated++
	}

	fmt.Printf("done: added=%d updated=%d skipped=%d failed=%d\n", added, updated, skipped, failed)
	if failed != 0 {
		os.Exit(1)
	}
}
