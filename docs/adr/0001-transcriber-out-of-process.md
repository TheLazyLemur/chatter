# The Transcriber runs out of process, behind a port

Phonon-2 ships only as a Python/MLX runtime (`fermion-research`); there is no C or Go runtime and the Core ML build is unreleased. So the Go app never links a speech model: it talks to one through the `core.Transcriber` port, by default over the OpenAI-compatible `/v1/audio/transcriptions` endpoint that `fermion serve phonon-2` exposes. Any server that speaks that shape (OpenAI, Groq, whisper.cpp, speaches) is a config change, and a different transport is one new adapter under `internal/provider/`.

## Consequences

- The app starts `fermion serve` itself when no server answers, and stops it on exit.
- Live preview uses Phonon's own WebSocket protocol (`/v1/audio/stream`), so it is a separate, optional port (`core.LivePreview`). The delivered text always comes from the Transcriber, never from the preview.
