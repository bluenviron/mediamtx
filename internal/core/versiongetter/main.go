// Package main contains an utility to get the server version
package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Golang version of git describe --tags
func gitDescribeTags(repo *git.Repository) (string, error) {
	head, err := repo.Head()
	if err != nil {
		return "", err
	}

	tagIterator, err := repo.Tags()
	if err != nil {
		return "", err
	}
	defer tagIterator.Close()

	tags := make(map[plumbing.Hash]*plumbing.Reference)

	err = tagIterator.ForEach(func(t *plumbing.Reference) error {
		if to, err2 := repo.TagObject(t.Hash()); err2 == nil {
			tags[to.Target] = t
		} else {
			tags[t.Hash()] = t
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	cIter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return "", err
	}

	i := 0

	for {
		commit, err := cIter.Next()
		if err != nil {
			return "", err
		}

		if str, ok := tags[commit.Hash]; ok {
			label := strings.TrimPrefix(string(str.Name()), "refs/tags/")

			if i != 0 {
				label += "-" + strconv.FormatInt(int64(i), 10) + "-" + head.Hash().String()[:8]
			}

			return label, nil
		}

		i++
	}
}

func do() error {
	log.Println("getting mediamtx version...")

	repo, err := git.PlainOpen("../..")
	if err != nil {
		return err
	}

	version, err := gitDescribeTags(repo)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	status, err := wt.Status()
	if err != nil {
		return err
	}

	if !status.IsClean() {
		version += "-dirty"
	}

	err = os.WriteFile("VERSION", []byte(version), 0o644)
	if err != nil {
		return err
	}

	log.Printf("ok (%s)", version)
	return nil
}

func main() {
	err := do()
	if err != nil {
		log.Printf("ERR: %v", err)
		os.Exit(1)
	}
}
