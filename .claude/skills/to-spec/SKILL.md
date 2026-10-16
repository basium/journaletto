---
name: to-spec
description: "Turn the current conversation (a /grill-me session on Requirement.md / TechSpec.md) into a PRD in .claude/PRD/<date-slug>/prd.md: no interview, just synthesis of what you've already discussed."
disable-model-invocation: true
---

<!-- Adapted from mattpocock/skills (MIT, see LICENSE): output goes to .claude/PRD/ instead of an issue tracker. -->

This skill takes the current conversation context and codebase understanding and produces a PRD. Do NOT interview the user; just synthesize what you already know.

**Where it goes (this project):** write it to `.claude/PRD/<YYYY-MM-DD>-<slug>/prd.md`, using the folder the Project Leader created at kickoff. Wherever the steps below say "publish it to the project issue tracker" or mention triage labels, write that file instead and put `Status: draft` at the top. Keep it as short as the module allows: the PRD is read sparingly (by the Project Leader, to create tasks and answer agents' questions). Use the vocabulary from `Requirement.md` and `TechSpec.md`; never edit those two files, they are human-owned.

## Process

1. Explore the repo to understand the current state of the codebase, if you haven't already. Use the project's domain glossary vocabulary throughout the spec, and respect any ADRs in the area you're touching.

2. Sketch out the seams at which you're going to test the feature. Existing seams should be preferred to new ones. Use the highest seam possible. If new seams are needed, propose them at the highest point you can. The fewer seams across the codebase, the better - the ideal number is one.

Check with the user that these seams match their expectations.

3. Write the spec using the template below, then publish it to the project issue tracker. Apply the `ready-for-agent` triage label - no need for additional triage.

<spec-template>

## Problem Statement

The problem that the user is facing, from the user's perspective.

## Solution

The solution to the problem, from the user's perspective.

## User Stories

A LONG, numbered list of user stories. Each user story should be in the format of:

1. As an <actor>, I want a <feature>, so that <benefit>

<user-story-example>
1. As a mobile bank customer, I want to see balance on my accounts, so that I can make better informed decisions about my spending
</user-story-example>

This list of user stories should be extremely extensive and cover all aspects of the feature.

## Implementation Decisions

A list of implementation decisions that were made. This can include:

- The modules that will be built/modified
- The interfaces of those modules that will be modified
- Technical clarifications from the developer
- Architectural decisions
- Schema changes
- API contracts
- Specific interactions

Do NOT include specific file paths or code snippets. They may end up being outdated very quickly.

Exception: if a prototype produced a snippet that encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it within the relevant decision and note briefly that it came from a prototype. Trim to the decision-rich parts, not a working demo, just the important bits.

## Testing Decisions

A list of testing decisions that were made. Include:

- A description of what makes a good test (only test external behavior, not implementation details)
- Which modules will be tested
- Prior art for the tests (i.e. similar types of tests in the codebase)

## Out of Scope

A description of the things that are out of scope for this spec.

## Further Notes

Any further notes about the feature.

</spec-template>
