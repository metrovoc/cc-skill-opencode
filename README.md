# cc-skill-opencode

Claude Code skills that enable multi-model AI as subagents via [OpenCode](https://opencode.ai).

## Features

- **Multi-turn conversation**: In-depth discussion with another model can help Claude make better decisions
- **Full-access subagents**: `--worktree` enables file editing for subagents

## Use Cases

- Ask Gemini Flash to read a large PDF
- Have Claude brainstorm with Gemini Pro
- Let Claude delegate large refactoring to GPT

## Skills

| Skill          | Use Case                  |
| -------------- | ------------------------- |
| `gemini-flash` | PDFs, images, quick tasks |
| `gemini-pro`   | Deep reasoning, research  |
| `gpt`          | Long coding tasks         |

Exact model versions are defined in `ocw/main.go` (`flashModel`, `proModel`, `gptModel`). Run `ocw list` or check source for current bindings.

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
