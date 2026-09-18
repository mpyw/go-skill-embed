---
name: example-adoption
description: Stand-in skill for the singlechecker example. A real linter ships the skill that explains how to read its diagnostics and what to do about each shape.
license: MIT
---

# Example adoption

A linter's skill earns its place by saying what the diagnostics mean and what
to do about each one, which is the part a README rarely covers well.

Keep it in the repository as `skills/<name>/SKILL.md`, embed the directory, and
the binary can install it wherever the user's agent reads from.
