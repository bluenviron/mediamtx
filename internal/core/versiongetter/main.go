// Package main contains an utility to get the server version
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Golang version of git describe --tags
func gitDescribeTags(repo *git.Repository) (string, error) {
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	tagIterator, err := repo.Tags()
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
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
		return "", fmt.Errorf("failed to iterate tags: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return "", fmt.Errorf("failed to get log: %w", err)
	}

	i := 0

	for {
		commit, err := cIter.Next()
		if err != nil {
			return "", fmt.Errorf("failed to get next commit: %w", err)
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

func tagFromGit() error {
	// [git.PlainOpen] uses a ChrootOS that limits filesystem access to the .git directory only.
	//
	// Unfortunately, this can cause issues with package build environments such as Arch Linux's,
	// where .git/objects/info/alternates points to a directory outside of the .git directory.
	//
	// To work around this, specify an AlternatesFS that allows access to the entire filesystem.
	dotGitAbs, _ := filepath.Abs("../../.git")
	storerFs := osfs.New(dotGitAbs, osfs.WithBoundOS())
	storer := filesystem.NewStorageWithOptions(storerFs, cache.NewObjectLRUDefault(), filesystem.Options{
		AlternatesFS: osfs.New("/", osfs.WithBoundOS()),
	})
	workTreeAbs, _ := filepath.Abs("../../")
	worktreeFs := osfs.New(workTreeAbs, osfs.WithBoundOS())
	repo, err := git.Open(storer, worktreeFs)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	version, err := gitDescribeTags(repo)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if !status.IsClean() {
		version += "-dirty"
	}

	err = os.WriteFile("VERSION", []byte(version), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}

	log.Printf("ok (%s)", version)
	return nil
}

func do() error {
	log.Println("getting mediamtx version...")

	err := tagFromGit()
	if err != nil {
		log.Println("WARN: cannot get tag from .git folder, using v0.0.0 as version")
		err = os.WriteFile("VERSION", []byte("v0.0.0"), 0o644)
		if err != nil {
			return fmt.Errorf("failed to write version file: %w", err)
		}
	}

	return nil
}

func main() {
	err := do()
	if err != nil {
		log.Printf("ERR: %v", err)
		os.Exit(1)
	}
}
