package ghaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/logging"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

type githubEvent struct {
	Before      string `json:"before"`
	After       string `json:"after"`
	PullRequest struct {
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

func resolveRepoRoot(getwd func() (string, error), lookupEnv func(string) (string, bool)) (string, string, error) {
	if workspace, ok := lookupEnv("GITHUB_WORKSPACE"); ok && strings.TrimSpace(workspace) != "" {
		return workspace, "GITHUB_WORKSPACE", nil
	}
	wd, err := getwd()
	if err != nil {
		return "", "", err
	}
	return wd, "cwd", nil
}

func (r baseRunner) resolveChangedPaths(changedOnly bool) ([]string, error) {
	if !changedOnly {
		return nil, nil
	}

	r.logger.Infof("resolving changed paths from GitHub event")
	eventName, eventPath, baseSHA, headSHA, err := r.resolveGitRange()
	if err != nil {
		return nil, err
	}
	r.logger.Infof("resolved git range for event %q from payload %q: %s..%s", eventName, eventPath, baseSHA, headSHA)

	changedPaths, err := r.deps.RunGitDiff(r.ctx, r.repoRoot, baseSHA, headSHA)
	if err != nil {
		return nil, err
	}
	r.logger.Infof("found %d changed path(s)", len(changedPaths))
	logging.LogList(r.logger, "changed paths", changedPaths)
	return changedPaths, nil
}

func (r baseRunner) resolveGitRange() (string, string, string, string, error) {
	eventName, _ := r.deps.LookupEnv("GITHUB_EVENT_NAME")
	eventPath, _ := r.deps.LookupEnv("GITHUB_EVENT_PATH")
	if strings.TrimSpace(eventPath) == "" {
		return "", "", "", "", errors.New("changed-only requires GITHUB_EVENT_PATH")
	}

	content, err := r.deps.ReadFile(eventPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read GitHub event payload: %w", err)
	}

	var payload githubEvent
	if err := json.Unmarshal(content, &payload); err != nil {
		return "", "", "", "", fmt.Errorf("decode GitHub event payload: %w", err)
	}

	switch eventName {
	case "pull_request", "pull_request_target":
		if payload.PullRequest.Base.SHA == "" || payload.PullRequest.Head.SHA == "" {
			return "", "", "", "", errors.New("pull_request event payload is missing base/head SHAs")
		}
		return eventName, eventPath, payload.PullRequest.Base.SHA, payload.PullRequest.Head.SHA, nil
	case "push":
		if payload.Before == "" || payload.After == "" {
			return "", "", "", "", errors.New("push event payload is missing before/after SHAs")
		}
		return eventName, eventPath, payload.Before, payload.After, nil
	default:
		return "", "", "", "", fmt.Errorf("changed-only is not supported for GitHub event %q", eventName)
	}
}

func changedPathsBetweenCommits(ctx context.Context, repoRoot string, baseSHA string, headSHA string) ([]string, error) {
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("open git repository: %w", err)
	}

	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, fmt.Errorf("load base commit %s: %w", baseSHA, err)
	}

	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, fmt.Errorf("load head commit %s: %w", headSHA, err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load base tree %s: %w", baseSHA, err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load head tree %s: %w", headSHA, err)
	}

	changes, err := object.DiffTreeContext(ctx, baseTree, headTree)
	if err != nil {
		return nil, fmt.Errorf("diff commits %s..%s: %w", baseSHA, headSHA, err)
	}

	changedSet := map[string]struct{}{}
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, fmt.Errorf("resolve change action for %s..%s: %w", baseSHA, headSHA, err)
		}
		if action == merkletrie.Delete {
			continue
		}

		path := change.To.Name
		if path == "" {
			path = change.From.Name
		}
		if path == "" {
			continue
		}
		changedSet[toSlash(path)] = struct{}{}
	}

	return sortedKeys(changedSet), nil
}
