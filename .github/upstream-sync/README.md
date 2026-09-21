# 自动同步上游

将 `Wei-Shaw/sub2api:main` 同步到 `foxtwobao/tokenone:main`。

## 运行方式

- 每天北京时间 06:23 检查（GitHub 的定时任务可能延迟），也可以在 Actions → **Sync upstream** → **Run workflow** 手动运行。
- 上游提交已包含在本仓库时，不创建 PR。
- 有新提交时，以该批上游 SHA 创建 `codex/sync-upstream/<sha>` 分支，并尝试合入 TokenOne 的 `main`。
- 无冲突时，创建同步 PR 并申请自动合并。GitHub 分支保护要求 CI、安全扫描、同步脚本测试通过，且分支包含最新 `main`。
- 有冲突时，撤销本地尝试的合并，以完整上游提交创建 PR。PR 正文列出冲突文件，添加 `upstream-sync-needs-human` 标签并提醒仓库所有者；不会提交冲突标记。
- CI 失败时停止自动合并，添加同一标签并提醒所有者。同一待处理状态不重复评论。
- 同步 PR 尚未处理完时不创建下一批，也不把新上游提交推入该 PR。人工提交会保留。若 `main` 前进且没有冲突，使用 GitHub 的 update-branch API 合入最新 `main`，以当前 PR SHA 防止并发覆盖。
- CI / Security Scan / Upstream sync tests 完成后重新检查已有同步 PR；冲突修复后也可以手动运行。
- 使用 merge commit 保留上游历史，不使用 squash、rebase、force push 或管理员绕过。
- 主动关闭某批 PR 后不会为同一上游 SHA 再建 PR。需要重试时重新打开原 PR；下一批上游提交仍会包含被跳过的变更。

## 一次性配置

### 1. 添加 PAT

在 GitHub 创建限定于 `foxtwobao/tokenone` 的 fine-grained PAT，配置：

| Repository permission | 权限 | 用途 |
| --- | --- | --- |
| Contents | Read and write | 推送同步分支、合并 PR |
| Pull requests | Read and write | 创建、更新、评论 PR，设置标签和自动合并 |
| Workflows | Read and write | 同步上游对 `.github/workflows/` 的更改 |
| Administration | Read-only | 验证分支保护，脚本不修改仓库设置 |
| Checks | Read-only | 读取 PR 检查结果 |

Metadata 的只读权限由 GitHub 自动附带。如果使用 classic PAT，则需要 `repo` 和 `workflow` scopes。

将 PAT 保存到仓库 **Settings → Secrets and variables → Actions → New repository secret**，名称为 `UPSTREAM_SYNC_TOKEN`。不要将 token 写入代码、日志或聊天。到期后在同一 Secret 更新。

使用 PAT 是为了让创建 PR 和合并产生的事件继续触发 CI，以及现有的 Docker Hub 发布工作流。默认 `GITHUB_TOKEN` 的 push 不会继续触发发布。

### 2. 配置仓库合并规则

在 Settings → General → Pull Requests 中开启：

- Allow merge commits
- Allow auto-merge

在 Settings → Branches 为 `main` 配置分支保护：

- Require status checks to pass before merging
- Require branches to be up to date before merging
- Do not allow bypassing the above settings（规则同样约束管理员）
- Required checks：`shell`、`test`、`frontend`、`golangci-lint`、`release-helpers`、`backend-security`、`frontend-security`、`upstream-sync-tests`

这些检查来自 GitHub Actions。首次安装时先让 `upstream-sync-tests` 在 GitHub 跑一次，便于在界面中选择。

若要求人工审批，则 PR 会等待审批，不会完全无人值守。需要自动同步时不设置必需审批人数。上述保护也会约束其他对 `main` 的修改，后续开发建议走 PR。

脚本在推送或申请合并前验证这些条件；配置缺失或读取失败会停止，不会降级为直接推送 `main`。

## 人工解决冲突

在自己的工作副本执行（把 `<branch>` 替换为 PR 分支）：

```bash
git fetch tokenone
git switch --track tokenone/<branch>
git merge tokenone/main
# 编辑冲突文件并验证功能
git add <已解决的文件>
git commit
git push tokenone HEAD
```

如果已经有该分支，先切换并拉取最新内容。工作流不会覆盖你的修复。修复完成且检查通过后，自动合并重新启用；合并到 `main` 将触发现有 Docker 镜像发布。

## 本地验证

```bash
python3 -B -m unittest discover -s .github/upstream-sync -p 'test_*.py' -v
```

测试使用临时本地 Git 仓库和模拟 GitHub API，不访问或修改远程仓库。不要直接在日常工作区运行 `sync.py`；它是有远程写入行为的 Actions 入口。

同步入口始终从本仓库的 `main` 运行，合并试验在临时 worktree 中进行，不执行同步分支中的代码。上游工作流的变更也属于同步范围；测试不能保证所有定制行为兼容，IDONE 等定制逻辑应持续保留回归测试。
