package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

func (app *application) pullPosts() error {
	postsDir := app.config.Blog.PostDir
	newDir := false
	if _, err := os.Stat(postsDir); os.IsNotExist(err) {
		newDir = true
	} else if err != nil {
		return fmt.Errorf("inspect posts directory: %w", err)
	}

	auth, err := ssh.NewPublicKeysFromFile("git", app.config.Github.PrivateKey, "")
	if err != nil {
		return err
	}

	if newDir {
		_, err := git.PlainClone(postsDir, false, &git.CloneOptions{
			URL:      app.config.Github.URL,
			Progress: os.Stdout,
			Auth:     auth,
		})
		if err != nil {
			return err
		}
	} else {
		repository, err := git.PlainOpen(postsDir)
		if err != nil {
			return fmt.Errorf("open posts repository: %w", err)
		}
		worktree, err := repository.Worktree()
		if err != nil {
			return fmt.Errorf("open posts worktree: %w", err)
		}
		err = worktree.Pull(&git.PullOptions{RemoteName: "origin", Auth: auth, Progress: os.Stdout})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("pull posts: %w", err)
		}
	}
	return nil
}
