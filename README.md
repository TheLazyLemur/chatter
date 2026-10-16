# Chatter

A Superwhisper-style dictation app for macOS, written in Go with SDL2. Press a hotkey, speak, and the
text is pasted into the app you were using. Speech recognition runs locally on
[Phonon-2](https://github.com/fermionresearch/phonon) (Fermion Research, 164 MB, English). Optional
rewriting goes through any OpenAI-compatible chat model.

## Quick start

```sh
brew install sdl2 sdl2_ttf ffmpeg     # ffmpeg is only for dropped files
make setup                            # Phonon-2 into .venv (about 170 MB)
make run
```

On first launch Chatter writes `~/Library/Application Support/Chatter/settings.json` and starts
`fermion serve phonon-2` itself. Grant **Microphone** and **Accessibility** access to the app
you launch it from. Accessibility is what lets Chatter paste and read selected text.

## Features

| | |
|---|---|
| Record hotkey | `⌃⌥Space`. Tap to start and tap again to finish, or hold it for push-to-talk. The hold threshold can be adjusted. |
| Cancel | `Esc` while recording. It is only bound during a recording. |
| Overlay | A floating pill with a live waveform, a timer, the mode, a **live preview** of the text, the processing stage, then the outcome. It never takes focus. |
| Paste | Output goes into the app that was frontmost when you started, and the previous clipboard is restored afterwards. It can be set to copy only. |
| Modes | Voice (raw), Message, Email, Note, Super and Prompt. You can create, edit and delete them in the Modes tab. Each mode has its own instructions, the context it reads (target app, selected text, clipboard), extra vocabulary, an optional hotkey of its own, and a list of apps. |
| Picking a mode | The mode you pick is used everywhere. Pick **Auto** to let the app decide instead: Slack gets Message, Ghostty gets Prompt, and other apps get the first mode. A mode's own hotkey overrides either choice. |
| Vocabulary | Sent to Phonon as hotwords, and given to the rewriter as exact spellings. |
| Replacements | Spoken phrases become fixed text. `new line` and `new paragraph` are set up by default. |
| History | Every dictation is kept with its audio. You can search, copy, play back, delete, or **re-run it in another mode**. |
| Files | Drop audio or video files on the window to transcribe them, or use `chatter-cli transcribe`. |
| Menu bar | 🎙 / 🔴 / ⏳ show the state. The menu lets you pick the mode, start and stop, paste the last dictation, or quit. |
| Other hotkeys | `⌃⌥M` switches to the next mode, `⌃⌥V` pastes the last dictation again, `⌃⌥O` opens the window. |
| Sound cues | Cues play on start, stop, cancel and error. They can be turned off. |
| Settings | Settings can be changed in the window or in the JSON file. Edits to the file are reloaded while the app runs. |

```sh
chatter-cli transcribe -mode note meeting.m4a   # any format ffmpeg reads
chatter-cli history -q invoice -n 5
chatter-cli modes
chatter-cli doctor                               # checks the transcriber, rewriter and ffmpeg
```

## Swapping the model

The speech model sits behind one port, `core.Transcriber`. Adapters live in `internal/provider/`,
and `internal/wiring` chooses one from `transcriber.kind`:

- `"http"` (the default) works with any OpenAI-compatible `/v1/audio/transcriptions` server.
  You change `base_url` and `model` to use a different one:
  - Phonon: `http://127.0.0.1:8000/v1`, `phonon-2`
  - OpenAI: `https://api.openai.com/v1`, `gpt-4o-transcribe`, and `api_key_env: "OPENAI_API_KEY"`
  - a whisper.cpp server or speaches
  Set `autostart: []` when Chatter shouldn't launch Phonon itself.
- `"cli"` runs `fermion transcribe` once per dictation, with no server. Each run loads the model, so it's slower.

A new kind of model (an in-process engine, say) is a new type with one `Transcribe` method plus a
case in `wiring.NewSpeech`. Live preview has its own optional port, `core.LivePreview`, and the
pasted text always comes from the Transcriber. See `docs/adr/0001`.

The rewriter works the same way. `rewriter.base_url` and `rewriter.model` point at any
OpenAI-compatible chat API: Ollama, LM Studio, OpenAI, OpenRouter. Leave `base_url` empty to turn
rewriting off.

## Layout

```
cmd/chatter            desktop app (composition root: cmd/chatter/app)
cmd/chatter-cli        command line (composition root: cmd/chatter-cli/cli)
internal/core          domain: modes, dictation state machine, pipeline, ports, fakes
internal/provider/*    Transcriber and Rewriter adapters (openaiaudio, openaichat, fermion)
internal/{sdlaudio,desktop,history,audiofile,config}   other outbound adapters
internal/{gui,hotkey}  inbound adapters: SDL windows and menu bar, global hotkeys
pkg/                   domain-free libraries (wav, clock, test tiers)
```

Vocabulary is defined in `CONTEXT.md`. Tests are `make test/unit` and `make test/integration`.

## Known limits

- macOS on Apple silicon only: the hotkeys, paste and menu bar use Cocoa and Carbon, and Phonon-2 needs MLX.
- Phonon-2 is English only.
- Carbon cannot see whether another app also owns a hotkey. If your record hotkey opens something else (⌥Space opens Raycast), choose a different one.
- Clipboard restore keeps plain text only.
