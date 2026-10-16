# Chatter

Voice dictation for the desktop: speak, and the words land as text in whatever app you were using.

## Language

### Capturing speech

**Dictation**:
One cycle from pressing the record hotkey to text arriving in the target app.
_Avoid_: Session, take

**Recording**:
The audio captured during one Dictation, or decoded from a dropped file.
_Avoid_: Clip, sample

**Push-to-talk**:
Holding the record hotkey for longer than the hold threshold; releasing it ends the Dictation.
_Avoid_: Hold mode

**Toggle**:
Tapping the record hotkey; the next tap ends the Dictation.

**Live preview**:
The provisional text shown while a Dictation is still recording. It is never delivered.
_Avoid_: Partial transcript, interim

### Turning speech into text

**Transcriber**:
The speech model that turns a Recording into a Transcript.
_Avoid_: Engine, ASR, model (on its own)

**Transcript**:
The words the Transcriber heard, after Replacements, before any Mode rewrites them.
_Avoid_: Raw text

**Vocabulary**:
Names and terms the Transcriber should favour when the audio is close to them.
_Avoid_: Hotwords, dictionary, custom words

**Replacement**:
A rule that swaps a spoken phrase for fixed text in the Transcript.
_Avoid_: Snippet, substitution

**Mode**:
A named set of instructions that shapes a Transcript into Output for one kind of writing: a message, an email, a note.
_Avoid_: Preset, profile, style

**Picked mode**:
The Mode the user chose. It is used for every Dictation unless a Mode's own hotkey was pressed.
_Avoid_: Default mode, active mode

**Auto**:
A Picked mode choice that lets the Target app decide: the first Mode listing that app, else the first Mode.
_Avoid_: Auto-activation, smart mode

**Rewriter**:
The language model that applies a Mode's instructions to a Transcript.
_Avoid_: LLM, AI, post-processor

**Output**:
The text that is delivered: the Transcript itself when the Mode has no instructions, otherwise the Rewriter's result.
_Avoid_: Result, final text

**Context**:
What the user was looking at when the Dictation started: the target app, selected text, clipboard.
_Avoid_: Screen context, environment

### Delivering text

**Target app**:
The app that was frontmost when the Dictation started, and the one that receives the Output.
_Avoid_: Active window, focused app

**Delivery**:
Putting the Output into the Target app, by pasting it or by leaving it on the clipboard.

**History entry**:
A stored Dictation: its Recording, Transcript, Output and Mode.
_Avoid_: Log, record
