# Example Tools (External Dependencies)

These tool scripts require additional nlink-jp CLI tools to be built and installed.

## web-search.sh

Agentic web search via Vertex AI Gemini with Google Search Grounding.

**Requires:**
- [gem-search](https://github.com/nlink-jp/gem-search) — `make build && cp dist/gem-search ~/bin/`
- Vertex AI credentials — `gcloud auth application-default login`

## generate-image.sh

Image generation via Vertex AI Gemini.

**Requires:**
- [gem-image](https://github.com/nlink-jp/gem-image) — `make build && cp dist/gem-image ~/bin/`
- Vertex AI credentials — `gcloud auth application-default login`

## Installation

Copy the desired scripts to your tools directory:

```bash
cp web-search.sh ~/Library/Application\ Support/shell-agent/tools/
cp generate-image.sh ~/Library/Application\ Support/shell-agent/tools/
```

Then restart Shell Agent or click "Restart Tools" in Settings.
