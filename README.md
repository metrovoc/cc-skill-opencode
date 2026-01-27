# cc-skill-opencode

Claude Code skills for multi-model AI via [OpenCode](https://opencode.ai).

## Skills

| Skill          | Model          | Use Case                                        |
| -------------- | -------------- | ----------------------------------------------- |
| `gemini-flash` | Gemini 3 Flash | PDFs, images, quick tasks                       |
| `gemini-pro`   | Gemini 3 Pro   | Deep reasoning, research                        |
| `gpt`          | GPT-5.2 Codex  | Long coding tasks (supports worktree isolation) |

## Install

**As a Claude Code plugin**

```bash
/plugin marketplace add metrovoc/cc-skill-opencode
/plugin install opencode@cc-skill-opencode
```

**From source**

```bash
# Build ocw wrapper
cd ocw && go build -o ~/.local/bin/ocw .

# Link skills
ln -s $(pwd)/skills/* ~/.claude/skills/
```

## Design

- **Explicit session management**: `new` returns hash, `chat` requires hash
- **Minimal command for agents**: Handle argument parsing etc. through wrapper
- **Safe input**: Prompts via stdin (no shell escaping issues)
- **Worktree isolation**: `--worktree` for safe code edits
