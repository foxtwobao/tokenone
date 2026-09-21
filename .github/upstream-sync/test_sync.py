import copy
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import Mock

import sync


class GitTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.upstream = root / "upstream"
        self.origin = root / "origin.git"
        self.checkout = root / "checkout"
        self.run_git("init", "-b", "main", str(self.upstream))
        self.run_git("-C", str(self.upstream), "config", "user.name", "test")
        self.run_git("-C", str(self.upstream), "config", "user.email", "test@example.com")
        self.commit(self.upstream, "common", "base\n")
        self.run_git("clone", "--bare", str(self.upstream), str(self.origin))
        self.run_git("clone", str(self.origin), str(self.checkout))
        self.run_git("-C", str(self.checkout), "config", "user.name", "test")
        self.run_git("-C", str(self.checkout), "config", "user.email", "test@example.com")
        self.git = sync.Git(self.checkout, upstream=str(self.upstream))

    def run_git(self, *args):
        result = subprocess.run(["git", *args], text=True, capture_output=True, check=True)
        return result.stdout.strip()

    def commit(self, directory, path, text):
        (directory / path).write_text(text)
        self.run_git("-C", str(directory), "add", path)
        self.run_git("-C", str(directory), "commit", "-m", "test change")
        return self.run_git("-C", str(directory), "rev-parse", "HEAD")

    def test_already_synced_with_local_customizations(self):
        self.commit(self.checkout, "custom", "IDONE customization\n")
        self.run_git("-C", str(self.checkout), "push", "origin", "main")
        self.git.fetch()
        self.assertTrue(self.git.contained())

    def test_clean_merge_preserves_both_histories_and_main(self):
        base = self.commit(self.checkout, "custom", "IDONE customization\n")
        self.run_git("-C", str(self.checkout), "push", "origin", "main")
        upstream = self.commit(self.upstream, "feature", "upstream feature\n")
        self.git.fetch()
        self.assertFalse(self.git.contained())
        branch = sync.PREFIX + upstream
        self.assertEqual([], self.git.publish(branch))
        self.assertEqual(base, self.run_git("--git-dir", str(self.origin), "rev-parse", "main"))
        for parent in (base, upstream):
            self.run_git("--git-dir", str(self.origin), "merge-base", "--is-ancestor", parent, branch)
        self.assertEqual("IDONE customization", self.run_git("--git-dir", str(self.origin), "show", f"{branch}:custom"))
        head = self.run_git("--git-dir", str(self.origin), "rev-parse", branch)
        self.assertEqual([], self.git.publish(branch))
        self.assertEqual(head, self.run_git("--git-dir", str(self.origin), "rev-parse", branch))
        self.assertEqual("", self.run_git("-C", str(self.checkout), "status", "--porcelain"))

    def test_conflict_publishes_upstream_without_markers_or_main_changes(self):
        base = self.commit(self.checkout, "common", "our version\n")
        self.run_git("-C", str(self.checkout), "push", "origin", "main")
        upstream = self.commit(self.upstream, "common", "their version\n")
        self.git.fetch()
        branch = sync.PREFIX + upstream
        self.assertEqual(["common"], self.git.publish(branch))
        self.assertEqual(upstream, self.run_git("--git-dir", str(self.origin), "rev-parse", branch))
        self.assertEqual(base, self.run_git("--git-dir", str(self.origin), "rev-parse", "main"))
        self.assertEqual("their version", self.run_git("--git-dir", str(self.origin), "show", f"{branch}:common"))

    def test_existing_branch_with_human_commit_is_untouched(self):
        upstream = self.commit(self.upstream, "feature", "feature\n")
        self.git.fetch()
        branch = sync.PREFIX + upstream
        self.git.publish(branch)
        self.run_git("-C", str(self.checkout), "fetch", "origin", branch)
        self.run_git("-C", str(self.checkout), "checkout", "-b", "fix", "FETCH_HEAD")
        human = self.commit(self.checkout, "human", "manual fix\n")
        self.run_git("-C", str(self.checkout), "push", "origin", f"HEAD:{branch}")
        self.git.publish(branch)
        self.assertEqual(human, self.run_git("--git-dir", str(self.origin), "rev-parse", branch))


class AutomationTests(unittest.TestCase):
    def setUp(self):
        self.git = Mock()
        self.api = Mock()
        self.api.pulls.return_value = []
        self.api.protected_files.return_value = []
        self.pr = {
            "number": 1, "url": "https://github.com/foxtwobao/tokenone/pull/1",
            "headRefName": sync.PREFIX + "abc", "headRefOid": "abc",
            "isCrossRepository": False, "isDraft": False, "mergeable": "MERGEABLE",
            "mergeStateStatus": "BLOCKED", "labels": [], "statusCheckRollup": [],
            "autoMergeRequest": None,
        }
        self.api.view.return_value = self.pr

    def test_no_new_upstream_does_not_create_pr(self):
        self.git.contained.return_value = True
        sync.synchronize(self.git, self.api)
        self.api.create.assert_not_called()
        self.git.publish.assert_not_called()

    def test_protected_change_blocks_even_when_all_checks_pass(self):
        self.api.protected_files.return_value = ["backend/internal/server/router.go"]
        self.pr["statusCheckRollup"] = [{"name": "test", "conclusion": "SUCCESS"}]
        sync.maintain(self.api, 1)
        self.api.auto.assert_not_called()
        self.api.label.assert_called_once_with(1, sync.REVIEW)
        self.assertIn("backend/internal/server/router.go", self.api.comment.call_args.args[1])

    def test_protected_change_cancels_existing_auto_merge_without_repeated_comment(self):
        self.api.protected_files.return_value = ["frontend/src/router/index.ts"]
        self.pr.update(labels=[{"name": sync.REVIEW}], autoMergeRequest={"enabledAt": "now"})
        sync.maintain(self.api, 1)
        self.api.auto.assert_called_once_with(self.pr, False)
        self.api.comment.assert_not_called()

    def test_removing_review_label_does_not_approve_protected_changes(self):
        self.api.protected_files.return_value = ["backend/internal/server/routes/auth.go"]
        sync.maintain(self.api, 1)
        self.api.label.assert_called_once_with(1, sync.REVIEW)
        self.api.auto.assert_not_called()

    def test_reverted_protected_change_can_merge(self):
        self.pr["labels"] = [{"name": sync.REVIEW}]
        sync.maintain(self.api, 1)
        self.api.label.assert_called_once_with(1, sync.REVIEW, remove=True)
        self.api.auto.assert_called_once_with(self.pr, True)

    def test_file_api_failure_disables_auto_merge(self):
        self.api.protected_files.side_effect = RuntimeError("API unavailable")
        self.pr["autoMergeRequest"] = {"enabledAt": "now"}
        with self.assertRaisesRegex(RuntimeError, "API unavailable"):
            sync.maintain(self.api, 1)
        self.api.auto.assert_called_once_with(self.pr, False)

    def test_new_clean_pr_requests_gated_merge(self):
        self.git.contained.return_value = False
        self.git.fetch.return_value = "abc"
        self.git.publish.return_value = []
        self.api.create.return_value = self.pr
        sync.synchronize(self.git, self.api)
        self.git.publish.assert_called_once_with(sync.PREFIX + "abc")
        self.api.auto.assert_called_once_with(self.pr, True)

    def test_existing_pr_is_preserved(self):
        self.api.pulls.return_value = [self.pr]
        sync.synchronize(self.git, self.api)
        self.git.fetch.assert_not_called()
        self.git.publish.assert_not_called()
        self.api.create.assert_not_called()

    def test_conflict_disables_auto_merge_and_requests_help(self):
        self.pr.update(mergeable="CONFLICTING", autoMergeRequest={"enabledAt": "now"})
        sync.maintain(self.api, 1)
        self.api.auto.assert_called_once_with(self.pr, False)
        self.api.label.assert_called_once_with(1, sync.ATTENTION)
        self.api.comment.assert_called_once()

    def test_known_local_conflict_handles_github_unknown_state(self):
        self.pr["mergeable"] = "UNKNOWN"
        sync.maintain(self.api, 1, ["file"])
        self.api.label.assert_called_once_with(1, sync.ATTENTION)
        self.api.auto.assert_not_called()

    def test_failed_checks_do_not_merge_or_repeat_notifications(self):
        self.pr.update(statusCheckRollup=[{"name": "test", "conclusion": "FAILURE"}],
                       labels=[{"name": sync.ATTENTION}])
        sync.maintain(self.api, 1)
        self.api.auto.assert_not_called()
        self.api.comment.assert_not_called()

    def test_manual_fix_reenables_auto_merge(self):
        self.pr.update(labels=[{"name": sync.ATTENTION}],
                       statusCheckRollup=[{"name": "test", "conclusion": "SUCCESS"}])
        sync.maintain(self.api, 1)
        self.api.label.assert_called_once_with(1, sync.ATTENTION, remove=True)
        self.api.auto.assert_called_once_with(self.pr, True)

    def test_behind_pr_updates_with_head_guard(self):
        self.pr["mergeStateStatus"] = "BEHIND"
        sync.maintain(self.api, 1)
        self.api.api.assert_called_once_with("pulls/1/update-branch", "PUT", {"expected_head_sha": "abc"})
        self.api.auto.assert_not_called()

    def test_unknown_and_draft_never_merge(self):
        for state in ({"mergeable": "UNKNOWN"}, {"isDraft": True}):
            self.api.reset_mock()
            self.api.view.return_value = dict(self.pr, **state)
            sync.maintain(self.api, 1)
            self.api.auto.assert_not_called()

    def test_closed_revision_is_not_reopened(self):
        self.git.fetch.return_value = "abc"
        self.git.contained.return_value = False
        self.api.pulls.side_effect = [[], [self.pr]]
        sync.synchronize(self.git, self.api)
        self.git.publish.assert_not_called()

    def test_check_completion_does_not_start_a_new_batch(self):
        sync.synchronize(self.git, self.api, existing_only=True)
        self.git.fetch.assert_not_called()

    def test_multiple_active_prs_fail_closed(self):
        self.api.pulls.return_value = [self.pr, dict(self.pr, number=2)]
        with self.assertRaisesRegex(RuntimeError, "Multiple"):
            sync.synchronize(self.git, self.api)
        self.git.publish.assert_not_called()

    def test_missing_protection_prevents_mutation(self):
        self.api.verify_settings.side_effect = RuntimeError("missing protection")
        with self.assertRaises(RuntimeError):
            sync.synchronize(self.git, self.api)
        self.git.fetch.assert_not_called()
        self.api.auto.assert_not_called()


class ProtectedFilesTests(unittest.TestCase):
    def test_modified_deleted_and_renamed_files_across_pages(self):
        api = sync.GitHub()
        files = [{"filename": f"unrelated/{i}"} for i in range(100)]
        protected = sorted(sync.PROTECTED_FILES)
        api.api = Mock(side_effect=[files, [
            {"filename": protected[0], "status": "modified"},
            {"filename": protected[1], "status": "removed"},
            {"filename": "new/location.go", "previous_filename": protected[2], "status": "renamed"},
            {"filename": protected[3], "previous_filename": "old/location.go", "status": "renamed"},
        ]])
        self.assertEqual(protected, api.protected_files(42))
        self.assertEqual("pulls/42/files?per_page=100&page=2", api.api.call_args.args[0])

    def test_fork_only_additions_do_not_require_review(self):
        api = sync.GitHub()
        api.api = Mock(return_value=[{"filename": path} for path in [
            "backend/internal/server/middleware/oidc_only.go",
            "backend/internal/handler/auth_oidc_redirect.go",
            "frontend/src/views/auth/CasdoorLoginView.vue",
            ".github/upstream-sync/sync.py",
        ]])
        self.assertEqual([], api.protected_files(1))

    def test_file_limit_fails_closed(self):
        api = sync.GitHub()
        api.api = Mock(return_value=[{"filename": f"unrelated/{i}"} for i in range(100)])
        with self.assertRaisesRegex(RuntimeError, "3,000-file limit"):
            api.protected_files(1)


class SettingsTests(unittest.TestCase):
    def test_required_checks_strict_and_admin_enforcement(self):
        valid = {
            "required_status_checks": {"strict": True, "checks": [{"context": c} for c in sync.CHECKS]},
            "enforce_admins": {"enabled": True},
        }
        variants = [valid, {}, copy.deepcopy(valid), copy.deepcopy(valid), copy.deepcopy(valid)]
        variants[2]["required_status_checks"]["strict"] = False
        variants[3]["enforce_admins"]["enabled"] = False
        variants[4]["required_status_checks"]["checks"].pop()
        for index, protection in enumerate(variants):
            with self.subTest(index=index):
                api = sync.GitHub()
                api.api = Mock(side_effect=[{"allow_auto_merge": True, "allow_merge_commit": True}, protection])
                if index == 0:
                    api.verify_settings()
                else:
                    with self.assertRaises(RuntimeError):
                        api.verify_settings()


if __name__ == "__main__":
    unittest.main()
