# cc-skill-opencode

Claude Code skills that enable multi-model AI as subagents via [OpenCode](https://opencode.ai).

## Features

- **Multi-turn conversation**: In-depth discussion with another model can help Claude make better decisions
- **Full-access subagents**: `--worktree` enables file editing for subagents

## Use Cases

- Ask Gemini 3 Flash to read a large PDF
- Have Claude brainstorm with Gemini 3 Pro
- Let Claude delegate large refactoring to GPT-5.2

## Skills

| Skill          | Model          | Use Case                  |
| -------------- | -------------- | ------------------------- |
| `gemini-flash` | Gemini 3 Flash | PDFs, images, quick tasks |
| `gemini-pro`   | Gemini 3 Pro   | Deep reasoning, research  |
| `gpt`          | GPT-5.2 Codex  | Long coding tasks         |

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
- **Minimal CLI for agents**: Wrapper handles argument parsing
- **Safe input**: Prompts via stdin (no shell escaping issues)
- **Worktree isolation**: `--worktree` for safe code edits
