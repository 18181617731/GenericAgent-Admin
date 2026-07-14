# Repository Agent Instructions

## 提交前同步上游

- `origin` 指向当前 fork：`18181617731/GenericAgent-Admin`。
- `upstream` 指向源项目：`Fwind43/GenericAgent-Admin`。
- 每次提交代码前必须先执行 `git fetch upstream main`，检查 `upstream/main` 是否有新提交。
- 若上游有更新，先执行 `git merge -X ours upstream/main` 合并到当前本地分支，再进行本地提交。
- 合并发生冲突时优先保留当前本地修改；仍未自动解决的冲突必须逐项检查并按本地行为优先处理。
- 上游合并完成后必须运行与影响范围匹配的测试、lint 和 build；验证通过后才可提交并推送到 `origin`。
