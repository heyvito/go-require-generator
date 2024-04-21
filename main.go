package main

import (
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	app := &cli.App{
		Name:      "grg",
		Usage:     "Obtains a require statement based on a git repository",
		ArgsUsage: "repo-url [repo-url [repo-url [...]]]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Usage:   "Prints out every command and result",
				Aliases: []string{"v"},
			},
		},
		Action: func(ctx *cli.Context) error {
			if ctx.NArg() == 0 {
				return cli.ShowAppHelp(ctx)
			}

			gitPath, err := exec.LookPath("git")
			if err != nil {
				return cli.Exit("Could not find git in your PATH", 1)
			}

			var results []string
			errorList := map[string]string{}

			for _, v := range ctx.Args().Slice() {
				r, err := processRepo(ctx.IsSet("verbose"), v, gitPath)
				if err != nil {
					errorList[v] = err.Error()
				} else {
					results = append(results, r)
				}
			}

			fmt.Println()
			if len(errorList) > 0 {
				fmt.Println("The following errors were found:")
				for repo, err := range errorList {
					fmt.Printf("  %s: %s\n", repo, err)
				}
				fmt.Println()
			}

			if len(results) > 0 {
				for _, v := range results {
					fmt.Println(v)
				}
			}

			if len(errorList) > 0 {
				return cli.Exit("One or more repositories could not be processed", 1)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type GitExecError struct {
	StdOut        string
	StdErr        string
	Status        int
	OriginalError error
}

func (e GitExecError) Error() string {
	return fmt.Sprintf("Failed to execute git command. Exit code %d: %s", e.Status, e.OriginalError)
}

func gitFail(stdout, stderr strings.Builder, err error) error {
	status := -1
	var e *exec.ExitError
	if errors.As(err, &e) {
		status = e.ExitCode()
	}
	return GitExecError{
		OriginalError: err,
		StdOut:        stdout.String(),
		StdErr:        stderr.String(),
		Status:        status,
	}
}

func cloneRepo(verbose bool, name, into, gitExec string, ssh bool) error {
	args := []string{"clone", "--depth=1", "--bare"}
	hostPath := strings.SplitN(name, "/", 2)
	host, path := hostPath[0], hostPath[1]
	splitPath := strings.Split(path, "/")
	if len(splitPath) > 2 {
		path = strings.Join(splitPath[0:2], "/")
	}

	if ssh {
		args = append(args, fmt.Sprintf("git@%s:%s", host, path))
	} else {
		args = append(args, fmt.Sprintf("https://%s/%s", host, path))
	}

	args = append(args, "repo")

	if verbose {
		fmt.Printf("verbose: Executing %s %s\n", gitExec, strings.Join(args, " "))
	}

	cmd := exec.Command(gitExec, args...)
	cmd.Dir = into

	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if verbose {
			fmt.Printf("verbose: Error executing:\n")
			lines := strings.Split(stdout.String(), "\n")
			lines = append(lines, strings.Split(stderr.String(), "\n")...)
			for i, v := range lines {
				lines[i] = "        " + v
			}
			fmt.Printf("%s\n", strings.Join(lines, "\n"))
		}
		return gitFail(stdout, stderr, err)
	}

	return nil
}

func getLastTag(verbose bool, gitExec, dir string) (bool, string) {
	cmd := exec.Command(gitExec, "describe", "--tags", "--abbrev=0")
	if verbose {
		fmt.Printf("verbose: Executing %s %s\n", gitExec, strings.Join(cmd.Args, " "))
	}
	cmd.Dir = filepath.Join(dir, "repo")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if verbose {
			fmt.Printf("verbose: Error executing:\n")
			lines := strings.Split(stdout.String(), "\n")
			lines = append(lines, strings.Split(stderr.String(), "\n")...)
			for i, v := range lines {
				lines[i] = "        " + v
			}
			fmt.Printf("%s\n", strings.Join(lines, "\n"))
		}
		return false, ""
	}

	return true, strings.TrimSpace(stdout.String())
}

func getLastCommit(verbose bool, gitExec, dir string) (bool, string, string) {
	cmd := exec.Command(gitExec, "log", "-1", "--date=format-local:%Y%m%d%H%M%S", "--format=%cd")
	if verbose {
		fmt.Printf("verbose: Executing %s %s\n", gitExec, strings.Join(cmd.Args, " "))
	}
	cmd.Dir = filepath.Join(dir, "repo")
	cmd.Env = append(os.Environ(), "TZ=GMT")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if verbose {
			fmt.Printf("verbose: Error executing:\n")
			lines := strings.Split(stdout.String(), "\n")
			lines = append(lines, strings.Split(stderr.String(), "\n")...)
			for i, v := range lines {
				lines[i] = "        " + v
			}
			fmt.Printf("%s\n", strings.Join(lines, "\n"))
		}
		return false, "", ""
	}

	ts := strings.TrimSpace(stdout.String())

	cmd = exec.Command(gitExec, "rev-parse", "--short=12", "HEAD")
	if verbose {
		fmt.Printf("verbose: Executing %s %s\n", gitExec, strings.Join(cmd.Args, " "))
	}
	cmd.Dir = filepath.Join(dir, "repo")
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		if verbose {
			fmt.Printf("verbose: Error executing:\n")
			lines := strings.Split(stdout.String(), "\n")
			lines = append(lines, strings.Split(stderr.String(), "\n")...)
			for i, v := range lines {
				lines[i] = "        " + v
			}
			fmt.Printf("%s\n", strings.Join(lines, "\n"))
		}
		return false, "", ""
	}

	commit := strings.TrimSpace(stdout.String())

	return true, commit, ts
}

func processRepo(verbose bool, path string, gitPath string) (string, error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	defer func(path string) { _ = os.RemoveAll(path) }(dir)

	err = cloneRepo(verbose, path, dir, gitPath, true)
	if err != nil {
		if verbose {
			fmt.Printf("verbose: Error cloning repository: %s\n", err)
		}
		err = cloneRepo(verbose, path, dir, gitPath, false)
		if err != nil {
			return "", fmt.Errorf("failed clonning via HTTPS and SSH. Check you have access to the repository")
		}
	}

	hasTag, tagName := getLastTag(verbose, gitPath, dir)
	if hasTag && strings.HasPrefix(tagName, "v") {
		return fmt.Sprintf("require %s %s", path, tagName), nil
	}

	ok, commit, ts := getLastCommit(verbose, gitPath, dir)
	if ok {
		return fmt.Sprintf("require %s v0.0.0-%s-%s", path, ts, commit), nil
	}

	return "", fmt.Errorf("failed obtaining information from clonned repository")
}
