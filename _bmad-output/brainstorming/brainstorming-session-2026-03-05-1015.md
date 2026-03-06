---
stepsCompleted: [1]
inputDocuments: []
session_topic: 'Go LLM wrapper library for personal projects (Ollama + OpenRouter)'
session_goals: 'Design a clean, ergonomic Go library API with environment-based provider selection, sensible defaults, support for user/system prompts, and multi-modal input/generation'
selected_approach: ''
techniques_used: []
ideas_generated: []
context_file: ''
---

# Brainstorming Session Results

**Facilitator:** Neal
**Date:** 2026-03-05

## Session Overview

**Topic:** Go LLM wrapper library — a shared personal library enabling LLM prompt queries via Ollama or OpenRouter
**Goals:** Design the library API, configuration semantics, and architecture with sensible defaults and clean ergonomics

### Session Setup

**Core Requirements Captured:**
- Two backends: Ollama (local) and OpenRouter (cloud)
- Auto-detection: if Ollama environment vars present → use Ollama; if OpenRouter API key present → use OpenRouter
- `MODEL` env var overrides the model selection
- Hardcoded default model when no override is set
- Support for basic user prompts and system prompts
- Designed for reuse across personal Go projects
- Multi-modal support: image (and potentially other media) input alongside text prompts

