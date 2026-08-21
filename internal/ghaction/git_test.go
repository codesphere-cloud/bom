package ghaction

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("resolveGitRange", func() {
	It("resolves base and head SHAs for a pull request event", func() {
		eventPath := filepath.Join(GinkgoT().TempDir(), "event.json")
		content := `{"pull_request":{"base":{"sha":"base123"},"head":{"sha":"head456"}}}`
		Expect(os.WriteFile(eventPath, []byte(content), 0o600)).To(Succeed())

		runner := baseRunner{
			deps: Dependencies{
				ReadFile: os.ReadFile,
				LookupEnv: func(key string) (string, bool) {
					switch key {
					case "GITHUB_EVENT_NAME":
						return "pull_request", true
					case "GITHUB_EVENT_PATH":
						return eventPath, true
					default:
						return "", false
					}
				},
			},
		}

		eventName, gotEventPath, base, head, err := runner.resolveGitRange()
		Expect(err).NotTo(HaveOccurred())
		Expect(eventName).To(Equal("pull_request"))
		Expect(gotEventPath).To(Equal(eventPath))
		Expect(base).To(Equal("base123"))
		Expect(head).To(Equal("head456"))
	})
})

var _ = Describe("resolveRepoRoot", func() {
	It("prefers GITHUB_WORKSPACE when set", func() {
		repoRoot, source, err := resolveRepoRoot(func() (string, error) {
			return "/tmp/cwd", nil
		}, func(key string) (string, bool) {
			if key == "GITHUB_WORKSPACE" {
				return "/github/workspace", true
			}
			return "", false
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repoRoot).To(Equal("/github/workspace"))
		Expect(source).To(Equal("GITHUB_WORKSPACE"))
	})

	It("falls back to the current working directory", func() {
		repoRoot, source, err := resolveRepoRoot(func() (string, error) {
			return "/tmp/cwd", nil
		}, func(key string) (string, bool) {
			return "", false
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repoRoot).To(Equal("/tmp/cwd"))
		Expect(source).To(Equal("cwd"))
	})
})

var _ = Describe("changedPathsBetweenCommits", func() {
	It("returns the sorted set of paths changed between two commits", func() {
		repoRoot := GinkgoT().TempDir()

		repo, err := git.PlainInit(repoRoot, false)
		Expect(err).NotTo(HaveOccurred())

		writeFile := func(path string, content string) {
			absolutePath := filepath.Join(repoRoot, filepath.FromSlash(path))
			Expect(os.MkdirAll(filepath.Dir(absolutePath), 0o755)).To(Succeed())
			Expect(os.WriteFile(absolutePath, []byte(content), 0o600)).To(Succeed())
		}

		commitAll := func(message string, paths ...string) string {
			worktree, err := repo.Worktree()
			Expect(err).NotTo(HaveOccurred())
			for _, path := range paths {
				_, err := worktree.Add(path)
				Expect(err).NotTo(HaveOccurred())
			}

			hash, err := worktree.Commit(message, &git.CommitOptions{
				Author: &object.Signature{
					Name:  "Test",
					Email: "test@example.com",
					When:  time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
				},
			})
			Expect(err).NotTo(HaveOccurred())

			return hash.String()
		}

		writeFile("charts/api/Chart.yaml", "name: api\n")
		writeFile("README.md", "before\n")
		baseSHA := commitAll("base", "charts/api/Chart.yaml", "README.md")

		writeFile("charts/api/values.yaml", "replicas: 2\n")
		writeFile("README.md", "after\n")
		headSHA := commitAll("head", "charts/api/values.yaml", "README.md")

		got, err := changedPathsBetweenCommits(context.Background(), repoRoot, baseSHA, headSHA)
		Expect(err).NotTo(HaveOccurred())

		want := []string{"README.md", "charts/api/values.yaml"}
		Expect(slices.Equal(got, want)).To(BeTrue())
	})
})
