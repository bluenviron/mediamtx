//go:build enable_linters

package docsorder

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const repoPath = "../../.."

var numberedFileRe = regexp.MustCompile(`^(\d+)-`)

type numberedFile struct {
	name   string
	numStr string
	num    int
}

func TestDocsOrder(t *testing.T) {
	entries, err := os.ReadDir(repoPath + "/docs")
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := fmt.Sprintf("%s/docs/%s", repoPath, entry.Name())

		files, err := os.ReadDir(dirPath)
		require.NoError(t, err)

		var numbered []numberedFile
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			m := numberedFileRe.FindStringSubmatch(f.Name())
			if m == nil {
				continue
			}
			num, err2 := strconv.Atoi(m[1])
			if err2 != nil {
				t.Errorf("docs/%s/%s: cannot parse numeric prefix %q", entry.Name(), f.Name(), m[1])
				continue
			}
			numbered = append(numbered, numberedFile{
				name:   f.Name(),
				numStr: m[1],
				num:    num,
			})
		}

		if len(numbered) == 0 {
			continue
		}

		sort.Slice(numbered, func(i, j int) bool {
			return numbered[i].num < numbered[j].num
		})

		usesPadding := len(numbered) >= 10

		for i, nf := range numbered {
			expected := i + 1

			if nf.num != expected {
				t.Errorf("docs/%s/%s: expected number %d, got %d", entry.Name(), nf.name, expected, nf.num)
			}

			if usesPadding {
				expectedStr := fmt.Sprintf("%02d", expected)
				if nf.numStr != expectedStr {
					t.Errorf("docs/%s/%s: expected zero-padded prefix %q, got %q", entry.Name(), nf.name, expectedStr, nf.numStr)
				}
			} else {
				if strings.HasPrefix(nf.numStr, "0") {
					t.Errorf("docs/%s/%s: unexpected zero-padded prefix %q (directory has fewer than 10 files)", entry.Name(), nf.name, nf.numStr)
				}
			}
		}
	}
}
