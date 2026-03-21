//go:build enable_linters

package docslinks

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

const (
	repoPath      = "../../.."
	docsPath      = "docs/**/*.md"
	additionalDoc = "README.md"
)

type docFile struct {
	anchors map[string]struct{}
}

type markdownLink struct {
	line   int
	target string
}

func collectDocFile(docPath string) (docFile, error) {
	anchors := make(map[string]struct{})
	anchorCounts := make(map[string]int)

	_, err := scanMarkdown(docPath, func(lineNum int, line string) {
		heading, ok := parseHeading(line)
		if !ok {
			return
		}

		anchor := slugifyHeading(heading)
		if anchor == "" {
			return
		}

		count := anchorCounts[anchor]
		anchorCounts[anchor] = count + 1
		if count > 0 {
			anchor = anchor + "-" + strconv.Itoa(count)
		}

		anchors[anchor] = struct{}{}
	})
	if err != nil {
		return docFile{}, err
	}

	return docFile{anchors: anchors}, nil
}

func collectLinks(docPath string) ([]markdownLink, error) {
	var links []markdownLink

	_, err := scanMarkdown(docPath, func(lineNum int, line string) {
		for _, target := range extractInlineLinks(line) {
			links = append(links, markdownLink{line: lineNum, target: target})
		}
	})

	return links, err
}

func scanMarkdown(docPath string, cb func(lineNum int, line string)) (map[string]struct{}, error) {
	file, err := os.Open(docPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	activeFence := ""
	lineNum := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if fence, ok := fenceDelimiter(line); ok {
			if activeFence == "" {
				activeFence = fence
			} else if activeFence == fence {
				activeFence = ""
			}
			continue
		}
		if activeFence != "" {
			continue
		}

		cb(lineNum, line)
	}

	return nil, scanner.Err()
}

func fenceDelimiter(line string) (string, bool) {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if trimmed == "" {
		return "", false
	}

	r, _ := utf8.DecodeRuneInString(trimmed)
	if r != '`' && r != '~' {
		return "", false
	}

	count := 0
	for _, candidate := range trimmed {
		if candidate != r {
			break
		}
		count++
	}
	if count < 3 {
		return "", false
	}

	return strings.Repeat(string(r), count), true
}

func parseHeading(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}

	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level == len(trimmed) || trimmed[level] != ' ' {
		return "", false
	}

	heading := strings.TrimSpace(trimmed[level:])
	heading = strings.TrimRight(heading, " #")
	return heading, heading != ""
}

func slugifyHeading(heading string) string {
	var b strings.Builder
	prevHyphen := false

	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevHyphen = false

		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 && !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}

		case r == '/':
			if b.Len() > 0 && !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}

		case unicode.IsPunct(r) || unicode.IsSymbol(r):
		}
	}

	return strings.Trim(b.String(), "-")
}

func extractInlineLinks(line string) []string {
	var out []string

	for i := 0; i < len(line); i++ {
		if line[i] != '[' || isImageLink(line, i) {
			continue
		}

		closeLabel := strings.IndexByte(line[i:], ']')
		if closeLabel < 0 {
			continue
		}
		closeLabel += i
		if closeLabel+1 >= len(line) || line[closeLabel+1] != '(' {
			continue
		}

		closeTarget := strings.IndexByte(line[closeLabel+2:], ')')
		if closeTarget < 0 {
			continue
		}
		closeTarget += closeLabel + 2

		target := strings.TrimSpace(line[closeLabel+2 : closeTarget])
		if target != "" {
			out = append(out, target)
		}

		i = closeTarget
	}

	return out
}

func isImageLink(line string, i int) bool {
	return i > 0 && line[i-1] == '!'
}

func splitLinkTarget(target string) (string, string) {
	file, anchor, _ := strings.Cut(target, "#")
	return file, anchor
}

func isInternalDocLink(targetFile string, targetAnchor string) bool {
	if targetFile == "" {
		return targetAnchor != ""
	}

	lower := strings.ToLower(targetFile)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") {
		return false
	}

	return strings.HasSuffix(lower, ".md")
}

func toRepoPath(p string) string {
	rel, err := filepath.Rel(repoPath, p)
	if err != nil {
		panic(err)
	}
	return path.Clean(filepath.ToSlash(rel))
}

func TestDocsLinks(t *testing.T) {
	docPaths, err := filepath.Glob(repoPath + "/" + docsPath)
	require.NoError(t, err)
	docPaths = append(docPaths, repoPath+"/"+additionalDoc)

	docs := make(map[string]docFile)
	for _, docPath := range docPaths {
		doc, err := collectDocFile(docPath)
		require.NoError(t, err)
		docs[toRepoPath(docPath)] = doc
	}

	for _, docPath := range docPaths {
		links, err := collectLinks(docPath)
		require.NoError(t, err)

		sourceDocPath := toRepoPath(docPath)
		for _, link := range links {
			targetFile, targetAnchor := splitLinkTarget(link.target)
			if !isInternalDocLink(targetFile, targetAnchor) {
				continue
			}

			resolvedFile := sourceDocPath
			if targetFile != "" {
				resolvedFile = path.Clean(path.Join(path.Dir(sourceDocPath), targetFile))
			}

			targetDoc, ok := docs[resolvedFile]
			if !ok {
				t.Errorf("%s:%d: link target %q does not exist", sourceDocPath, link.line, link.target)
				continue
			}

			if targetAnchor != "" {
				if _, ok := targetDoc.anchors[targetAnchor]; !ok {
					t.Errorf("%s:%d: anchor %q in link target %q does not exist", sourceDocPath, link.line, targetAnchor, link.target)
				}
			}
		}
	}
}
