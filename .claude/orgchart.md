<!-- Read only by the Project Leader (root session) when spawning agents. Edit freely. -->

# Org chart

| Role | Agent name | Model | Effort | Max in parallel | Notes |
|---|---|---|---|---|---|
| Project Leader | (root session) | session model | session effort | 1 | Switch to `opus` for PRD/architecture work |
| Developer | `developer` | sonnet | medium | 3 | Each runs in its own worktree |
| Reviewer | `reviewer` | sonnet | low | 3 | ≤ 3 comments per PR |
| DevOps & Architect | `devops-architect` | sonnet | medium | 1 | Environment changes collide, so one at a time |
| Explorer | `explorer` | haiku | low | 2 | Read-only fact finding |

The PL passes the Model and Effort above when spawning each agent; they override the agent file defaults.
