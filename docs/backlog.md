# Backlog

## 1. Prompt mode invents requirements

Saying "Hello World, this is a test." in Ghostty came out as a made-up spec: "Goal: Create a simple Hello World program… output exactly: Hello World… keep the implementation minimal". None of that was said.

Cause: Prompt's instructions tell the Rewriter to turn every Transcript into "a clear instruction for an AI coding assistant: the goal first, then constraints". So a sentence that isn't a request still becomes one, and the gaps get filled in.

Fix: reword the default Prompt instructions so the Rewriter only restructures what was said. It must never add goals or constraints, and if the Transcript isn't a request it should only be cleaned up. Check with a non-request ("this is a test") and a real request.

Workaround today: pick Voice, or any mode other than Auto, so Ghostty no longer switches to Prompt.

## 2. Auto needs an explicit fallback mode

With Auto picked, an app that no Mode lists gets the first Mode in `modes`. The only way to change that is to reorder `settings.json`; the mode editor can't reorder.

Fix: add a "When no app matches, use: [mode]" setting for Auto, shown in the Modes tab and stored in the settings file. Mode selection (`core.SelectMode`) uses it in place of `Modes[0]`. Settings validation rejects a fallback that names no Mode, and deleting that Mode resets the fallback.
