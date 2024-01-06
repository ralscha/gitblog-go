package main

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"os"
	"os/exec"
)

func (app *application) pullPosts() error {
	postsDir := app.config.Blog.PostDir
	newDir := false
	if _, err := os.Stat(postsDir); os.IsNotExist(err) {
		err = os.Mkdir(postsDir, 0755)
		if err != nil {
			return err
		}
		newDir = true
	}

	auth, err := ssh.NewPublicKeysFromFile("git", app.config.Github.PrivateKey, "")
	if err != nil {
		return err
	}

	if newDir {
		_, err := git.PlainClone(postsDir, false, &git.CloneOptions{
			URL:      app.config.Github.Url,
			Progress: os.Stdout,
			Auth:     auth,
		})
		if err != nil {
			return err
		}
	} else {
		cmd := exec.Command("git", "pull")
		cmd.Dir = postsDir

		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("error pulling posts: %w", err)
		}
	}
	return nil
}
