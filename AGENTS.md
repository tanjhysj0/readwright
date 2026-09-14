# AGENTS.md

## Agent skills

### Issue tracker

GitHub Issues，通过 `gh` CLI 操作。见 `docs/agents/issue-tracker.md`。

### Triage labels

使用五个中文分诊标签：待分诊 / 待补充信息 / 待代理处理 / 待人工处理 / 不予修复。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局（根目录 `CONTEXT.md` + `docs/adr/`）。见 `docs/agents/domain.md`。

### 铁律
一个issue完成之后，`git commit` 之前需要向我确认，我确认之后方可提交和关闭issue。