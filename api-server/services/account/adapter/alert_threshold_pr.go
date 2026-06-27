package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"nudgebee/services/common"
	"nudgebee/services/config"
)

// AlertThresholdPRParams describes a single alert-threshold rule-file change to land via PR.
// Unlike rightsizing PRs (which resolve the repo from a k8s workload's annotations), an alert
// rule has no workload, so the repo/branch/file location are supplied explicitly by the caller.
type AlertThresholdPRParams struct {
	Provider         string // "github" (gitlab: see below)
	IntegrationName  string // integration (provider_config) name for credentials
	Org              string
	Repo             string
	Branch           string
	FilePath         string // path of the rule file in the repo
	OldContent       string // exact substring to replace (e.g. the old PromQL expr)
	NewContent       string // replacement (the rewritten PromQL expr)
	AlertName        string
	RecommendationID string // unique id; used for the branch name
	ReferenceLink    string
	ResolverType     string
}

// RaiseAlertThresholdPR checks out the repo, replaces the rule's threshold expression in the
// target file, commits, and opens a PR. Returns the PR URL. It reuses the existing GitOps
// helpers (checkoutCodeRepo / raisePrForCodeRepo) so credential handling and PR creation stay
// consistent with the rightsizing flow.
func RaiseAlertThresholdPR(ctx AccountAdapterContext, p AlertThresholdPRParams) (string, error) {
	if p.Provider != "" && p.Provider != "github" {
		return "", fmt.Errorf("alert-threshold PR is currently supported only for GitHub integrations (got %q)", p.Provider)
	}
	if p.IntegrationName == "" {
		return "", fmt.Errorf("git integration name is required")
	}
	if p.OldContent == "" || p.OldContent == p.NewContent {
		return "", fmt.Errorf("no rule change to write")
	}

	_, _, username, password, err := getGitCredentials(ctx, p.IntegrationName)
	if err != nil {
		return "", err
	}

	gitDetail := gitDetailFromDeployment{
		Org:        p.Org,
		Repo:       p.Repo,
		BaseBranch: p.Branch,
		Token:      password,
		Username:   username,
		FilePath:   p.FilePath,
	}

	dir, err := checkoutCodeRepo(ctx, ApplyRecommendationRequest{}, gitDetail)
	if err != nil {
		return "", err
	}
	defer func() {
		if rerr := os.RemoveAll(dir); rerr != nil {
			ctx.GetLogger().Error("alert threshold PR: failed to remove temp dir", "error", rerr)
		}
	}()

	branchName := "nb/threshold-" + p.RecommendationID
	if err := runGit(ctx.GetContext(), dir, "checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("create branch: %w", err)
	}

	safeFile, err := safeFilePath(dir, p.FilePath)
	if err != nil {
		return "", err
	}
	contentBytes, err := os.ReadFile(safeFile) //nolint:gosec // path validated by safeFilePath
	if err != nil {
		return "", fmt.Errorf("read rule file %q: %w", p.FilePath, err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, p.OldContent) {
		return "", fmt.Errorf("could not find the current rule expression in %q — the file may have changed or the path is wrong", p.FilePath)
	}
	updated := strings.Replace(content, p.OldContent, p.NewContent, 1)
	if updated == content {
		return "", fmt.Errorf("no change applied to %q", p.FilePath)
	}
	if err := os.WriteFile(safeFile, []byte(updated), 0o600); err != nil {
		return "", fmt.Errorf("write rule file: %w", err)
	}

	if err := runGit(ctx.GetContext(), dir, "add", "."); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if err := runGit(ctx.GetContext(), dir, "config", "user.email", config.Config.GitCommitNudgebeeEmail); err != nil {
		return "", fmt.Errorf("git config email: %w", err)
	}
	if err := runGit(ctx.GetContext(), dir, "config", "user.name", config.Config.GitCommitNudgebeeUser); err != nil {
		return "", fmt.Errorf("git config name: %w", err)
	}
	commitMsg := fmt.Sprintf("ci: tune alert threshold for %s", p.AlertName)
	if err := runGit(ctx.GetContext(), dir, "commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	title := fmt.Sprintf("Tune alert threshold: %s", p.AlertName)
	body := fmt.Sprintf("Nudgebee suggested tuning the threshold for alert **%s**.\n", p.AlertName)
	if p.ReferenceLink != "" {
		body += fmt.Sprintf("\nFor details, see [Nudgebee](%s).", p.ReferenceLink)
	}
	resolverType := p.ResolverType
	if resolverType == "" {
		resolverType = "AlertThreshold"
	}

	resp, err := raisePrForCodeRepo(ctx, dir, branchName, gitDetail, false, 0, body, resolverType, title)
	if err != nil {
		return "", err
	}

	prResponse := map[string]any{}
	if err := common.UnmarshalJson([]byte(resp), &prResponse); err != nil {
		return "", fmt.Errorf("parse PR response: %w", err)
	}
	if url, ok := prResponse["html_url"].(string); ok {
		return url, nil
	}
	return "", nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
