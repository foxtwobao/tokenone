"""Synchronize upstream through one PR; never force-push or bypass checks."""

import json
import os
from pathlib import Path
import subprocess
import tempfile


REPOSITORY = "foxtwobao/tokenone"
UPSTREAM = "https://github.com/Wei-Shaw/sub2api.git"
BASE = "main"
PREFIX = "codex/sync-upstream/"
LABEL = "upstream-sync"
ATTENTION = "upstream-sync-needs-human"
CHECKS = {
    "shell", "test", "frontend", "golangci-lint", "release-helpers",
    "backend-security", "frontend-security", "upstream-sync-tests",
}
FAILED = {"FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE"}


def command(*args, cwd=None, data=None, check=True):
    result = subprocess.run(args, cwd=cwd, input=data, text=True, capture_output=True)
    if check and result.returncode:
        # Do not print command arguments: callers may eventually carry credentials.
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or "Command failed")
    return result


class GitHub:
    def gh(self, *args, data=None):
        return command("gh", *args, data=data).stdout.strip()

    def api(self, path, method="GET", payload=None):
        args = ["api", f"repos/{REPOSITORY}/{path}" if path else f"repos/{REPOSITORY}", "--method", method]
        if payload is not None:
            args += ["--input", "-"]
        raw = self.gh(*args, data=json.dumps(payload) if payload is not None else None)
        return json.loads(raw) if raw else None

    def verify_settings(self):
        settings = self.api("")
        if not settings.get("allow_auto_merge") or not settings.get("allow_merge_commit"):
            raise RuntimeError("Enable auto-merge and merge commits; see upstream-sync/README.md")
        protection = self.api(f"branches/{BASE}/protection")
        required = protection.get("required_status_checks") or {}
        checks = required.get("checks") or []
        contexts = set(required.get("contexts") or []) | {c["context"] for c in checks}
        if not CHECKS <= contexts or not required.get("strict"):
            raise RuntimeError("All sync checks must be required and up-to-date before enabling auto-merge")
        if not (protection.get("enforce_admins") or {}).get("enabled"):
            raise RuntimeError("Branch protection must also apply to administrators")

    def pulls(self, state="open"):
        return json.loads(self.gh(
            "pr", "list", "--repo", REPOSITORY, "--base", BASE, "--state", state,
            "--limit", "100", "--json", "number,url,headRefName,isCrossRepository",
        ))

    def view(self, number):
        return json.loads(self.gh(
            "pr", "view", str(number), "--repo", REPOSITORY, "--json",
            "number,url,headRefName,headRefOid,mergeable,mergeStateStatus,isDraft,autoMergeRequest,labels,statusCheckRollup",
        ))

    def labels(self):
        for name, color, description in (
            (LABEL, "0366d6", "Automated upstream synchronization"),
            (ATTENTION, "d93f0b", "Upstream sync requires conflict resolution or CI fixes"),
        ):
            self.gh("label", "create", name, "--repo", REPOSITORY, "--color", color,
                    "--description", description, "--force")

    def create(self, branch, sha, conflicts):
        body = (
            f"Sync Wei-Shaw/sub2api@{sha} into `{BASE}` using a merge commit.\n\n"
            "Auto-merge waits for all required CI and security checks. "
            "This branch will not be overwritten by subsequent sync runs.\n\n"
            "If changes are needed, push fixes to this branch. For conflicts, merge "
            "the latest `main` into this branch, resolve the files, commit and push. "
            "The next check completion or sync run will re-evaluate auto-merge.\n"
        )
        if conflicts:
            body += "\nConflicting files at creation:\n\n" + "\n".join(f"- `{p}`" for p in conflicts) + "\n"
        return self.api("pulls", "POST", {
            "title": f"chore: sync upstream {sha[:12]}",
            "head": branch, "base": BASE, "body": body,
        })

    def label(self, number, name, remove=False):
        self.gh("pr", "edit", str(number), "--repo", REPOSITORY,
                "--remove-label" if remove else "--add-label", name)

    def comment(self, number, body):
        self.api(f"issues/{number}/comments", "POST", {"body": body})

    def auto(self, pr, enabled):
        args = ["pr", "merge", str(pr["number"]), "--repo", REPOSITORY]
        if enabled:
            args += ["--auto", "--merge", "--match-head-commit", pr["headRefOid"]]
        else:
            args += ["--disable-auto"]
        self.gh(*args)


class Git:
    def __init__(self, root, remote="origin", upstream=UPSTREAM):
        self.root = Path(root).resolve()
        self.remote = remote
        self.upstream = upstream

    def run(self, *args, cwd=None, check=True):
        return command("git", *args, cwd=cwd or self.root, check=check)

    def fetch(self):
        self.run("fetch", "--no-tags", self.remote, f"{BASE}:refs/sync/base")
        self.run("fetch", "--no-tags", self.upstream, f"{BASE}:refs/sync/upstream")
        return self.run("rev-parse", "refs/sync/upstream").stdout.strip()

    def contained(self):
        result = self.run("merge-base", "--is-ancestor", "refs/sync/upstream", "refs/sync/base", check=False)
        if result.returncode not in (0, 1):
            raise RuntimeError(result.stderr)
        return result.returncode == 0

    def remote_branch(self, branch):
        return bool(self.run("ls-remote", "--heads", self.remote, f"refs/heads/{branch}").stdout.strip())

    def publish(self, branch):
        # Recover a push that succeeded before PR creation failed. Never overwrite it.
        if self.remote_branch(branch):
            return []
        with tempfile.TemporaryDirectory(prefix="upstream-sync-") as temp:
            work = Path(temp) / "work"
            self.run("worktree", "add", "--detach", str(work), "refs/sync/upstream")
            try:
                result = self.run(
                    "-c", "user.name=tokenone-sync", "-c", "user.email=tokenone-sync@users.noreply.github.com",
                    "-c", "commit.gpgsign=false", "merge", "--no-ff", "--no-edit",
                    "refs/sync/base", cwd=work, check=False,
                )
                conflicts = []
                if result.returncode:
                    conflicts = self.run("diff", "--name-only", "--diff-filter=U", cwd=work).stdout.splitlines()
                    if not conflicts:
                        raise RuntimeError(result.stderr or result.stdout)
                    self.run("merge", "--abort", cwd=work)
                    # The upstream tip itself is a valid branch; GitHub will expose
                    # the conflict against main. Never commit conflict markers.
                self.run("push", self.remote, f"HEAD:refs/heads/{branch}", cwd=work)
                return conflicts
            finally:
                self.run("worktree", "remove", "--force", str(work))


def report(message):
    print(message)
    if path := os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(path, "a", encoding="utf-8") as file:
            file.write(message + "\n\n")


def maintain(api, number, known_conflicts=()):
    pr = api.view(number)
    labels = {label["name"] for label in pr["labels"]}
    failures = [c.get("name") or c.get("context") or "unknown check"
                for c in pr.get("statusCheckRollup") or []
                if (c.get("conclusion") or c.get("state")) in FAILED]
    conflicted = bool(known_conflicts) or pr["mergeable"] == "CONFLICTING"
    if conflicted or failures:
        if pr.get("autoMergeRequest"):
            api.auto(pr, False)
        if ATTENTION not in labels:
            api.label(number, ATTENTION)
            reason = "Merge conflicts require manual resolution." if conflicted else "CI failed: " + ", ".join(failures)
            api.comment(number, f"@{REPOSITORY.split('/')[0]} {reason}\n\n"
                        "Push fixes to this PR branch; automation will preserve your commits and re-check it.")
        report(f"Human action required: {pr['url']}")
        return
    if pr["mergeable"] != "MERGEABLE" or pr["isDraft"]:
        report(f"Waiting for GitHub mergeability or draft review: {pr['url']}")
        return
    if ATTENTION in labels:
        api.label(number, ATTENTION, remove=True)
    if pr.get("mergeStateStatus") == "BEHIND":
        # Merge new main commits without overwriting any human fixes. GitHub
        # rejects the request if the PR head changed concurrently.
        api.api(f"pulls/{number}/update-branch", "PUT", {"expected_head_sha": pr["headRefOid"]})
        report(f"Updated PR with latest main; waiting for fresh checks: {pr['url']}")
        return
    if not pr.get("autoMergeRequest"):
        api.auto(pr, True)
    report(f"Auto-merge requested; required checks gate merging: {pr['url']}")


def synchronize(git, api, existing_only=False):
    api.verify_settings()
    active = [p for p in api.pulls() if not p["isCrossRepository"] and p["headRefName"].startswith(PREFIX)]
    if len(active) > 1:
        raise RuntimeError("Multiple sync PRs are open; keep only one before resuming automation")
    if active:
        api.labels()
        maintain(api, active[0]["number"])
        return
    if existing_only:
        report("No open sync PR to update.")
        return
    sha = git.fetch()
    if git.contained():
        report("Already synchronized: upstream main is included in tokenone main.")
        return
    branch = PREFIX + sha
    # Respect a maintainer's explicit rejection instead of re-opening it daily.
    if any(p["headRefName"] == branch and not p["isCrossRepository"] for p in api.pulls("closed")):
        report("This upstream revision has a closed PR. Reopen it to retry, or wait for a new upstream revision.")
        return
    conflicts = git.publish(branch)
    api.labels()
    pr = api.create(branch, sha, conflicts)
    api.label(pr["number"], LABEL)
    maintain(api, pr["number"], conflicts)


if __name__ == "__main__":
    if os.environ.get("GITHUB_REPOSITORY") != REPOSITORY:
        raise SystemExit(f"This automation is only configured for {REPOSITORY}")
    synchronize(Git(Path.cwd()), GitHub(), os.environ.get("GITHUB_EVENT_NAME") == "workflow_run")
